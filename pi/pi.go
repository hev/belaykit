package pi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"belaykit"
)

// Verify Client implements belaykit.Agent.
var _ belaykit.Agent = (*Client)(nil)

// ErrCLINotFound indicates the pi CLI binary was not found on PATH.
var ErrCLINotFound = errors.New("pi CLI not found")

// ExitError wraps a non-zero exit from the pi CLI process.
type ExitError struct {
	Err    error
	Stderr string
}

func (e *ExitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("pi exited with error: %v, stderr: %s", e.Err, e.Stderr)
	}
	return fmt.Sprintf("pi exited with error: %v", e.Err)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

// Client wraps the pi CLI for headless JSON mode invocations.
type Client struct {
	executable      string
	defaultModel    string
	defaultProvider string
	defaultThinking string
	eventHandler    belaykit.EventHandler
	observability   belaykit.ObservabilityProvider
	tools           []string
	noTools         bool
	extensions      []string
	workDir         string
}

// NewClient creates a new pi CLI client.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		executable: "pi",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Run executes pi in JSON mode with the given prompt and returns the result.
func (c *Client) Run(ctx context.Context, prompt string, opts ...belaykit.RunOption) (belaykit.Result, error) {
	cfg := belaykit.NewRunConfig(opts...)

	model := c.defaultModel
	if cfg.Model != "" {
		model = cfg.Model
	}

	provider := c.defaultProvider
	thinking := c.defaultThinking

	// Build args: pi --mode json --no-session -p <prompt>
	args := []string{
		"--mode", "json",
		"--no-session",
		"-p", prompt,
	}

	if provider != "" {
		args = append(args, "--provider", provider)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if thinking != "" {
		args = append(args, "--thinking", thinking)
	}
	if cfg.SystemPrompt != "" {
		args = append(args, "--system-prompt", cfg.SystemPrompt)
	}

	// Tool configuration
	if c.noTools {
		args = append(args, "--no-tools")
	} else if len(c.tools) > 0 {
		args = append(args, "--tools", strings.Join(c.tools, ","))
	}

	// Extensions
	for _, ext := range c.extensions {
		args = append(args, "-e", ext)
	}

	cmd := exec.CommandContext(ctx, c.executable, args...)
	if c.workDir != "" {
		cmd.Dir = c.workDir
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return belaykit.Result{}, fmt.Errorf("creating stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return belaykit.Result{}, fmt.Errorf("creating stderr pipe: %w", err)
	}

	startTime := time.Now()

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return belaykit.Result{}, ErrCLINotFound
		}
		return belaykit.Result{}, &ExitError{Err: err}
	}

	handler := c.eventHandler
	if cfg.EventHandler != nil {
		handler = cfg.EventHandler
	}

	var resultText strings.Builder
	var numTurns int
	var totalInputTokens, totalOutputTokens int
	var totalCost float64
	var lastModel string
	var sessionID string
	var hadError bool
	var errorText string

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var event map[string]json.RawMessage
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		eventType := unquote(event["type"])
		rawLine := json.RawMessage(append([]byte(nil), line...))

		switch eventType {
		case "session":
			// Session header: {"type":"session","id":"..."}
			if id, ok := event["id"]; ok {
				sessionID = unquote(id)
			}
			if handler != nil {
				handler(belaykit.Event{
					Type:      belaykit.EventSystem,
					SessionID: sessionID,
					Subtype:   "init",
					RawJSON:   rawLine,
				})
			}

		case "turn_start":
			numTurns++
			if handler != nil {
				handler(belaykit.Event{
					Type:    belaykit.EventAssistantStart,
					RawJSON: rawLine,
				})
			}

		case "message_update":
			c.handleMessageUpdate(event, rawLine, handler, cfg.OutputStream, &resultText)

		case "tool_execution_start":
			if handler != nil {
				toolName := unquote(event["toolName"])
				toolID := unquote(event["toolCallId"])
				var toolInput json.RawMessage
				if args, ok := event["args"]; ok {
					toolInput = args
				}
				handler(belaykit.Event{
					Type:      belaykit.EventToolUse,
					ToolName:  toolName,
					ToolID:    toolID,
					ToolInput: toolInput,
					RawJSON:   rawLine,
				})
			}

		case "tool_execution_end":
			if handler != nil {
				toolID := unquote(event["toolCallId"])
				resultJSON := extractToolResultText(event["result"])
				handler(belaykit.Event{
					Type:    belaykit.EventToolResult,
					ToolID:  toolID,
					Text:    resultJSON,
					RawJSON: rawLine,
				})
			}

		case "message_end":
			// Extract usage from the completed message
			if msgRaw, ok := event["message"]; ok {
				in, out, cost, mdl := extractUsage(msgRaw)
				totalInputTokens += in
				totalOutputTokens += out
				totalCost += cost
				if mdl != "" {
					lastModel = mdl
				}
			}

		case "agent_end":
			duration := time.Since(startTime).Milliseconds()
			isError := hadError

			if handler != nil {
				evType := belaykit.EventResult
				text := resultText.String()
				if isError {
					evType = belaykit.EventResultError
					text = errorText
				}
				handler(belaykit.Event{
					Type:     evType,
					Text:     text,
					CostUSD:  totalCost,
					Duration: duration,
					NumTurns: numTurns,
					IsError:  isError,
					RawJSON:  rawLine,
				})
			}

			if c.observability != nil {
				resolvedModel := lastModel
				if resolvedModel == "" {
					resolvedModel = model
				}
				c.observability.RecordCompletion(belaykit.CompletionRecord{
					TraceID:      cfg.TraceID,
					SessionID:    sessionID,
					Prompt:       prompt,
					Response:     resultText.String(),
					Model:        resolvedModel,
					CostUSD:      totalCost,
					DurationMS:   duration,
					NumTurns:     numTurns,
					IsError:      hadError,
					InputTokens:  totalInputTokens,
					OutputTokens: totalOutputTokens,
				})
			}
		}
	}

	// Read stderr
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

	return belaykit.Result{Text: resultText.String()}, nil
}

// handleMessageUpdate processes message_update events, extracting text deltas
// and error events.
func (c *Client) handleMessageUpdate(
	event map[string]json.RawMessage,
	rawLine json.RawMessage,
	handler belaykit.EventHandler,
	outputStream interface{ Write([]byte) (int, error) },
	resultText *strings.Builder,
) {
	ameRaw, ok := event["assistantMessageEvent"]
	if !ok {
		return
	}

	var ame struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
	}
	if err := json.Unmarshal(ameRaw, &ame); err != nil {
		return
	}

	switch ame.Type {
	case "text_delta":
		resultText.WriteString(ame.Delta)
		if handler != nil {
			handler(belaykit.Event{
				Type:    belaykit.EventAssistant,
				Text:    ame.Delta,
				RawJSON: rawLine,
			})
		}
		if outputStream != nil {
			outputStream.Write([]byte(ame.Delta))
		}

	case "error":
		// The error reason/message is in the delta or a "reason" field
		if handler != nil {
			handler(belaykit.Event{
				Type:    belaykit.EventResultError,
				Text:    ame.Delta,
				IsError: true,
				RawJSON: rawLine,
			})
		}
	}
}

// extractUsage pulls token counts, cost, and model from a message_end message.
func extractUsage(msgRaw json.RawMessage) (input, output int, cost float64, model string) {
	var msg struct {
		Model string `json:"model"`
		Usage *struct {
			Input  int `json:"input"`
			Output int `json:"output"`
			Cost   *struct {
				Total float64 `json:"total"`
			} `json:"cost"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return
	}
	model = msg.Model
	if msg.Usage != nil {
		input = msg.Usage.Input
		output = msg.Usage.Output
		if msg.Usage.Cost != nil {
			cost = msg.Usage.Cost.Total
		}
	}
	return
}

// extractToolResultText extracts text content from a tool result payload.
func extractToolResultText(resultRaw json.RawMessage) string {
	if resultRaw == nil {
		return ""
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return string(resultRaw)
	}

	var parts []string
	for _, block := range result.Content {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// unquote removes JSON string quotes from a raw JSON value.
func unquote(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}
