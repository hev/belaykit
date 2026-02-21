package slack

import (
	"context"
	"fmt"

	"go-rack"
)

// HandlerOption configures the EventHandler returned by NewEventHandler.
type HandlerOption func(*handlerConfig)

// ErrorFormatter formats an error event into text + optional blocks.
type ErrorFormatter func(rack.Event) (string, []Block)

// ResultFormatter formats a result event into text + optional blocks.
type ResultFormatter func(rack.Event) (string, []Block)

type handlerConfig struct {
	agentName      string
	errorFormatter ErrorFormatter
	resultFormatter ResultFormatter
	ctx            context.Context
}

// WithHandlerAgentName sets the agent name included in default notification messages.
func WithHandlerAgentName(name string) HandlerOption {
	return func(cfg *handlerConfig) { cfg.agentName = name }
}

// WithErrorFormatter overrides the default error message formatter.
func WithErrorFormatter(fn ErrorFormatter) HandlerOption {
	return func(cfg *handlerConfig) { cfg.errorFormatter = fn }
}

// WithResultFormatter overrides the default result message formatter.
func WithResultFormatter(fn ResultFormatter) HandlerOption {
	return func(cfg *handlerConfig) { cfg.resultFormatter = fn }
}

// WithHandlerContext sets the context used for Slack API calls dispatched by
// the handler. Defaults to context.Background().
func WithHandlerContext(ctx context.Context) HandlerOption {
	return func(cfg *handlerConfig) { cfg.ctx = ctx }
}

// defaultErrorFormatter formats an error event for Slack.
func defaultErrorFormatter(agentName string) ErrorFormatter {
	return func(e rack.Event) (string, []Block) {
		prefix := "Error"
		if agentName != "" {
			prefix = fmt.Sprintf("[%s] Error", agentName)
		}
		text := fmt.Sprintf("%s: %s", prefix, e.Text)
		return text, nil
	}
}

// defaultResultFormatter formats a result event for Slack.
func defaultResultFormatter(agentName string) ResultFormatter {
	return func(e rack.Event) (string, []Block) {
		prefix := "Completed"
		if agentName != "" {
			prefix = fmt.Sprintf("[%s] Completed", agentName)
		}
		text := fmt.Sprintf("%s: turns=%d duration=%dms cost=$%.4f",
			prefix, e.NumTurns, e.Duration, e.CostUSD)
		return text, nil
	}
}

// NewEventHandler returns a rack.EventHandler that dispatches Slack
// notifications based on the notifier's EventConfig. Slack calls are made in
// goroutines so the handler never blocks the event stream.
//
// Composable with rack.NewLogger:
//
//	slackH := slack.NewEventHandler(notifier, ...)
//	logH := rack.NewLogger(os.Stderr, ...)
//	combined := func(e rack.Event) { logH(e); slackH(e) }
func NewEventHandler(notifier *Notifier, opts ...HandlerOption) rack.EventHandler {
	cfg := handlerConfig{
		ctx: context.Background(),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.errorFormatter == nil {
		cfg.errorFormatter = defaultErrorFormatter(cfg.agentName)
	}
	if cfg.resultFormatter == nil {
		cfg.resultFormatter = defaultResultFormatter(cfg.agentName)
	}

	events := notifier.cfg.Events
	sessionStarted := false

	return func(e rack.Event) {
		if !notifier.IsEnabled() {
			return
		}

		switch e.Type {
		case rack.EventSystem:
			if e.Subtype == "init" && events.OnStart && !sessionStarted {
				sessionStarted = true
				text := "Session started"
				if cfg.agentName != "" {
					text = fmt.Sprintf("[%s] Session started", cfg.agentName)
				}
				if e.SessionID != "" {
					text += fmt.Sprintf(" (session: %s)", e.SessionID)
				}
				go notifier.StartSession(cfg.ctx, text)
			}

		case rack.EventResultError:
			if events.OnError {
				text, blocks := cfg.errorFormatter(e)
				go notifier.Send(cfg.ctx, text, blocks...)
			}

		case rack.EventResult:
			if events.OnResult {
				text, blocks := cfg.resultFormatter(e)
				go notifier.EndSession(cfg.ctx, text, blocks...)
			}

		case rack.EventToolUse:
			if events.OnToolUse {
				text := fmt.Sprintf("Tool: %s", e.ToolName)
				if cfg.agentName != "" {
					text = fmt.Sprintf("[%s] Tool: %s", cfg.agentName, e.ToolName)
				}
				go notifier.Send(cfg.ctx, text)
			}
		}
	}
}
