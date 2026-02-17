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
)

// Client wraps the Claude CLI for headless mode invocations.
type Client struct {
	executable     string
	defaultModel   string
	eventHandler   EventHandler
	observability  ObservabilityProvider
}

// Result holds the response from a Claude CLI invocation.
type Result struct {
	Text string
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
func (c *Client) Run(ctx context.Context, prompt string, opts ...RunOption) (Result, error) {
	var cfg runConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	// Determine model (per-run overrides client default)
	model := c.defaultModel
	if cfg.model != "" {
		model = cfg.model
	}

	// Build args
	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
	}

	for _, tool := range cfg.allowedTools {
		args = append(args, "--allowedTools", tool)
	}

	if cfg.maxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", cfg.maxTurns))
	}

	if model != "" {
		args = append(args, "--model", model)
	}

	if cfg.systemPrompt != "" {
		args = append(args, "--system-prompt", cfg.systemPrompt)
	}

	cmd := exec.CommandContext(ctx, c.executable, args...)

	if cfg.maxOutputTokens > 0 {
		cmd.Env = append(os.Environ(), fmt.Sprintf("CLAUDE_CODE_MAX_OUTPUT_TOKENS=%d", cfg.maxOutputTokens))
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("creating stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return Result{}, ErrCLINotFound
		}
		return Result{}, &ExitError{Err: err}
	}

	// Determine event handler (per-run overrides client default)
	handler := c.eventHandler
	if cfg.eventHandler != nil {
		handler = cfg.eventHandler
	}

	// Parse streaming output
	var resultText string
	var sessionID string
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var event streamEvent
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
				handler(Event{
					Type:      EventSystem,
					SessionID: event.SessionID,
					Subtype:   event.Subtype,
					RawJSON:   rawLine,
				})
				if event.Subtype == "init" {
					handler(Event{Type: EventAssistantStart})
				}
			}
		case "assistant":
			if event.Message != nil {
				for _, block := range event.Message.Content {
					switch block.Type {
					case "text":
						if handler != nil {
							handler(Event{
								Type:    EventAssistant,
								Text:    block.Text,
								RawJSON: rawLine,
							})
						}
						if cfg.outputStream != nil {
							cfg.outputStream.Write([]byte(block.Text))
						}
					case "tool_use":
						if handler != nil {
							handler(Event{
								Type:      EventToolUse,
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
							handler(Event{
								Type:    EventToolResult,
								Text:    block.Content,
								ToolID:  block.ToolUseID,
								RawJSON: rawLine,
							})
						}
					}
				}
				if hadToolResults && handler != nil {
					handler(Event{Type: EventAssistantStart})
				}
			}
		case "result":
			resultText = event.Result
			evType := EventResult
			isError := event.IsError || event.Subtype == "error"
			if isError {
				evType = EventResultError
			}
			if handler != nil {
				handler(Event{
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
				c.observability.RecordCompletion(CompletionRecord{
					TraceID:    cfg.traceID,
					SessionID:  sessionID,
					Prompt:     prompt,
					Response:   event.Result,
					Model:      model,
					CostUSD:    event.CostUSD,
					DurationMS: event.DurationMS,
					NumTurns:   event.NumTurns,
					IsError:    isError,
				})
			}
		}
	}

	// Read any stderr
	var stderrBuf bytes.Buffer
	stderrBuf.ReadFrom(stderr)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return Result{}, &ExitError{
			Err:    err,
			Stderr: stderrBuf.String(),
		}
	}

	return Result{Text: resultText}, nil
}
