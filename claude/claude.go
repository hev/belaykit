package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"belaykit"
)

// Verify Client implements belaykit.Agent.
var _ belaykit.Agent = (*Client)(nil)

// ErrCLINotFound indicates the claude CLI binary was not found on PATH.
var ErrCLINotFound = errors.New("claude CLI not found")

// ExitError wraps a non-zero exit from the claude CLI process.
type ExitError struct {
	Err    error
	Stderr string
}

func (e *ExitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("claude exited with error: %v, stderr: %s", e.Err, e.Stderr)
	}
	return fmt.Sprintf("claude exited with error: %v", e.Err)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

// Client wraps the Claude CLI for headless mode invocations.
type Client struct {
	executable    string
	defaultModel  string
	eventHandler  belaykit.EventHandler
	observability belaykit.ObservabilityProvider
}

// NewClient creates a new Claude CLI client.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		executable: "claude",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Run executes the Claude CLI with the given prompt and returns the result.
func (c *Client) Run(ctx context.Context, prompt string, opts ...belaykit.RunOption) (belaykit.Result, error) {
	cfg := belaykit.NewRunConfig(opts...)

	// Determine model (per-run overrides client default)
	model := c.defaultModel
	if cfg.Model != "" {
		model = cfg.Model
	}

	// Build args
	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
	}

	for _, tool := range cfg.AllowedTools {
		args = append(args, "--allowedTools", tool)
	}

	for _, tool := range cfg.DisallowedTools {
		args = append(args, "--disallowedTools", tool)
	}

	if cfg.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", cfg.MaxTurns))
	}

	if model != "" {
		args = append(args, "--model", model)
	}

	if cfg.SystemPrompt != "" {
		args = append(args, "--system-prompt", cfg.SystemPrompt)
	}

	cmd := exec.CommandContext(ctx, c.executable, args...)

	if cfg.MaxOutputTokens > 0 {
		cmd.Env = append(os.Environ(), fmt.Sprintf("CLAUDE_CODE_MAX_OUTPUT_TOKENS=%d", cfg.MaxOutputTokens))
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return belaykit.Result{}, fmt.Errorf("creating stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return belaykit.Result{}, fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return belaykit.Result{}, ErrCLINotFound
		}
		return belaykit.Result{}, &ExitError{Err: err}
	}

	// Determine event handler (per-run overrides client default)
	handler := c.eventHandler
	if cfg.EventHandler != nil {
		handler = cfg.EventHandler
	}

	// Parse streaming output
	var resultText string
	var sessionID string
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var event belaykit.StreamEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		rawLine := json.RawMessage(line)

		switch event.Type {
		case "system":
			if event.SessionID != "" {
				sessionID = event.SessionID
			}
			if handler != nil {
				handler(belaykit.Event{
					Type:      belaykit.EventSystem,
					SessionID: event.SessionID,
					Subtype:   event.Subtype,
					RawJSON:   rawLine,
				})
				if event.Subtype == "init" {
					handler(belaykit.Event{Type: belaykit.EventAssistantStart})
				}
			}
		case "assistant":
			if event.Message != nil {
				for _, block := range event.Message.Content {
					switch block.Type {
					case "text":
						if handler != nil {
							handler(belaykit.Event{
								Type:    belaykit.EventAssistant,
								Text:    block.Text,
								RawJSON: rawLine,
							})
						}
						if cfg.OutputStream != nil {
							cfg.OutputStream.Write([]byte(block.Text))
						}
					case "tool_use":
						if handler != nil {
							handler(belaykit.Event{
								Type:      belaykit.EventToolUse,
								ToolName:  block.Name,
								ToolID:    block.ID,
								ToolInput: block.Input,
								RawJSON:   rawLine,
							})
						}
					}
				}
			}
		case "user":
			if event.Message != nil {
				var hadToolResults bool
				for _, block := range event.Message.Content {
					if block.Type == "tool_result" {
						hadToolResults = true
						if handler != nil {
							handler(belaykit.Event{
								Type:    belaykit.EventToolResult,
								Text:    block.Content,
								ToolID:  block.ToolUseID,
								RawJSON: rawLine,
							})
						}
					}
				}
				if hadToolResults && handler != nil {
					handler(belaykit.Event{Type: belaykit.EventAssistantStart})
				}
			}
		case "result":
			resultText = event.Result
			evType := belaykit.EventResult
			isError := event.IsError || event.Subtype == "error"
			if isError {
				evType = belaykit.EventResultError
			}
			if handler != nil {
				handler(belaykit.Event{
					Type:     evType,
					Text:     event.Result,
					Subtype:  event.Subtype,
					CostUSD:  event.CostUSD,
					Duration: event.DurationMS,
					NumTurns: event.NumTurns,
					IsError:  isError,
					RawJSON:  rawLine,
				})
			}
			if c.observability != nil {
				c.observability.RecordCompletion(belaykit.CompletionRecord{
					TraceID:      cfg.TraceID,
					SessionID:    sessionID,
					Prompt:       prompt,
					Response:     event.Result,
					Model:        model,
					CostUSD:      event.CostUSD,
					DurationMS:   event.DurationMS,
					NumTurns:     event.NumTurns,
					IsError:      isError,
				})
			}
		}
	}

	// Read any stderr
	var stderrBuf bytes.Buffer
	stderrBuf.ReadFrom(stderr)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return belaykit.Result{}, ctx.Err()
		}
		return belaykit.Result{}, &ExitError{
			Err:    err,
			Stderr: stderrBuf.String(),
		}
	}

	return belaykit.Result{Text: resultText}, nil
}
