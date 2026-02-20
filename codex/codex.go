package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	rack "go-rack"
)

// Verify Client implements rack.Agent.
var _ rack.Agent = (*Client)(nil)

// ErrCLINotFound indicates the codex CLI binary was not found on PATH.
var ErrCLINotFound = errors.New("codex CLI not found")

// UnsupportedOptionError indicates a run option not yet supported by codex.
type UnsupportedOptionError struct {
	Option string
}

func (e *UnsupportedOptionError) Error() string {
	return fmt.Sprintf("codex does not support option %s", e.Option)
}

// ExitError wraps a non-zero exit from the codex CLI process.
type ExitError struct {
	Err    error
	Stderr string
}

func (e *ExitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("codex exited with error: %v, stderr: %s", e.Err, e.Stderr)
	}
	return fmt.Sprintf("codex exited with error: %v", e.Err)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

// Client wraps the Codex CLI for non-interactive invocations.
type Client struct {
	executable    string
	defaultModel  string
	eventHandler  rack.EventHandler
	observability rack.ObservabilityProvider
}

// NewClient creates a new Codex CLI client.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		executable: "codex",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Run executes the Codex CLI with the given prompt and returns the result.
func (c *Client) Run(ctx context.Context, prompt string, opts ...rack.RunOption) (rack.Result, error) {
	cfg := rack.NewRunConfig(opts...)

	if err := validateRunConfig(cfg); err != nil {
		return rack.Result{}, err
	}

	model := c.defaultModel
	if cfg.Model != "" {
		model = cfg.Model
	}

	lastMsgFile, err := os.CreateTemp("", "go-rack-codex-last-message-*.txt")
	if err != nil {
		return rack.Result{}, fmt.Errorf("creating temp output file: %w", err)
	}
	lastMsgPath := lastMsgFile.Name()
	lastMsgFile.Close()
	defer os.Remove(lastMsgPath)

	composedPrompt := composePrompt(cfg.SystemPrompt, prompt)
	args := []string{"exec", "--json", "-o", lastMsgPath}
	if model != "" {
		args = append(args, "-m", model)
	}
	args = append(args, composedPrompt)

	cmd := exec.CommandContext(ctx, c.executable, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return rack.Result{}, fmt.Errorf("creating stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return rack.Result{}, fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return rack.Result{}, ErrCLINotFound
		}
		return rack.Result{}, &ExitError{Err: err}
	}

	handler := c.eventHandler
	if cfg.EventHandler != nil {
		handler = cfg.EventHandler
	}

	var stderrBuf bytes.Buffer
	state := runState{}
	lines := streamLines(stdout, stderr)
	for line := range lines {
		if !json.Valid(line.body) {
			if line.fromStderr {
				stderrBuf.Write(line.body)
				stderrBuf.WriteByte('\n')
			}
			continue
		}
		state.handleJSONLine(line.body, handler, cfg.OutputStream)
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return rack.Result{}, ctx.Err()
		}

		if handler != nil && !state.resultEmitted {
			handler(rack.Event{Type: rack.EventResultError, Text: state.lastError})
		}
		if c.observability != nil {
			c.observability.RecordCompletion(rack.CompletionRecord{
				TraceID:    cfg.TraceID,
				SessionID:  state.sessionID,
				Prompt:     prompt,
				Response:   state.lastError,
				Model:      model,
				CostUSD:    state.costUSD,
				DurationMS: state.durationMS,
				NumTurns:   state.numTurns,
				IsError:    true,
			})
		}

		return rack.Result{}, &ExitError{Err: err, Stderr: stderrBuf.String()}
	}

	resultText := state.assistantText.String()
	if data, err := os.ReadFile(lastMsgPath); err == nil && strings.TrimSpace(string(data)) != "" {
		resultText = string(data)
	}

	if handler != nil && !state.resultEmitted {
		handler(rack.Event{
			Type:     rack.EventResult,
			Text:     resultText,
			CostUSD:  state.costUSD,
			Duration: state.durationMS,
			NumTurns: state.numTurns,
		})
	}
	if c.observability != nil {
		c.observability.RecordCompletion(rack.CompletionRecord{
			TraceID:    cfg.TraceID,
			SessionID:  state.sessionID,
			Prompt:     prompt,
			Response:   resultText,
			Model:      model,
			CostUSD:    state.costUSD,
			DurationMS: state.durationMS,
			NumTurns:   state.numTurns,
			IsError:    false,
		})
	}

	return rack.Result{Text: resultText}, nil
}

func validateRunConfig(cfg rack.RunConfig) error {
	if cfg.MaxTurns > 0 {
		return &UnsupportedOptionError{Option: "WithMaxTurns"}
	}
	if cfg.MaxOutputTokens > 0 {
		return &UnsupportedOptionError{Option: "WithMaxOutputTokens"}
	}
	if len(cfg.AllowedTools) > 0 {
		return &UnsupportedOptionError{Option: "WithAllowedTools"}
	}
	if len(cfg.DisallowedTools) > 0 {
		return &UnsupportedOptionError{Option: "WithDisallowedTools"}
	}
	return nil
}

func composePrompt(systemPrompt, prompt string) string {
	if systemPrompt == "" {
		return prompt
	}
	return fmt.Sprintf("System instructions:\n%s\n\nUser prompt:\n%s", systemPrompt, prompt)
}

type runState struct {
	sessionID     string
	assistantText strings.Builder
	lastError     string
	costUSD       float64
	durationMS    int64
	numTurns      int
	resultEmitted bool
}

func (s *runState) handleJSONLine(line []byte, handler rack.EventHandler, outputStream io.Writer) {
	var payload map[string]any
	if err := json.Unmarshal(line, &payload); err != nil {
		return
	}

	eventType, _ := payload["type"].(string)
	raw := json.RawMessage(append([]byte(nil), line...))

	switch eventType {
	case "thread.started":
		if id, _ := payload["thread_id"].(string); id != "" {
			s.sessionID = id
			if handler != nil {
				handler(rack.Event{
					Type:      rack.EventSystem,
					SessionID: id,
					Subtype:   "init",
					RawJSON:   raw,
				})
			}
		}
	case "turn.started":
		s.numTurns++
		if handler != nil {
			handler(rack.Event{
				Type:    rack.EventAssistantStart,
				RawJSON: raw,
			})
		}
	case "turn.failed":
		msg := extractErrorMessage(payload)
		if msg == "" {
			msg = "codex run failed"
		}
		s.lastError = msg
		s.resultEmitted = true
		if handler != nil {
			handler(rack.Event{
				Type:    rack.EventResultError,
				Text:    msg,
				IsError: true,
				RawJSON: raw,
			})
		}
	}

	if v, ok := floatField(payload, "cost_usd"); ok {
		s.costUSD = v
	}
	if v, ok := int64Field(payload, "duration_ms"); ok {
		s.durationMS = v
	}

	if text := extractAssistantText(eventType, payload); text != "" {
		s.assistantText.WriteString(text)
		if outputStream != nil {
			outputStream.Write([]byte(text))
		}
		if handler != nil {
			handler(rack.Event{
				Type:    rack.EventAssistant,
				Text:    text,
				RawJSON: raw,
			})
		}
	}
}

func extractAssistantText(eventType string, payload map[string]any) string {
	if !looksLikeAssistantEvent(eventType) {
		return ""
	}

	for _, key := range []string{"delta", "text", "output_text"} {
		if v, ok := payload[key].(string); ok && v != "" {
			return v
		}
	}

	if msg, ok := payload["message"].(map[string]any); ok {
		if v, ok := msg["text"].(string); ok && v != "" {
			return v
		}
	}

	if item, ok := payload["item"].(map[string]any); ok {
		if v, ok := item["text"].(string); ok && v != "" {
			return v
		}
		if content, ok := item["content"].([]any); ok {
			for _, block := range content {
				if m, ok := block.(map[string]any); ok {
					if v, ok := m["text"].(string); ok && v != "" {
						return v
					}
				}
			}
		}
	}

	return ""
}

func looksLikeAssistantEvent(eventType string) bool {
	if eventType == "" {
		return false
	}
	if strings.Contains(eventType, "error") || strings.Contains(eventType, "failed") {
		return false
	}
	return strings.Contains(eventType, "assistant") ||
		strings.Contains(eventType, "message") ||
		strings.Contains(eventType, "output_text")
}

func extractErrorMessage(payload map[string]any) string {
	if msg, _ := payload["message"].(string); msg != "" {
		return msg
	}
	if errObj, ok := payload["error"].(map[string]any); ok {
		if msg, _ := errObj["message"].(string); msg != "" {
			return msg
		}
	}
	return ""
}

func floatField(payload map[string]any, key string) (float64, bool) {
	v, ok := payload[key]
	if !ok {
		return 0, false
	}
	num, ok := v.(float64)
	return num, ok
}

func int64Field(payload map[string]any, key string) (int64, bool) {
	v, ok := payload[key]
	if !ok {
		return 0, false
	}
	num, ok := v.(float64)
	if !ok {
		return 0, false
	}
	return int64(num), true
}

type streamLine struct {
	body       []byte
	fromStderr bool
}

func streamLines(stdout, stderr io.Reader) <-chan streamLine {
	out := make(chan streamLine, 64)
	var wg sync.WaitGroup
	read := func(r io.Reader, fromStderr bool) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			out <- streamLine{body: line, fromStderr: fromStderr}
		}
	}

	wg.Add(2)
	go read(stdout, false)
	go read(stderr, true)
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
