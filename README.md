# go-rack

<p align="center">
  <img src="go-rack.png" alt="go-rack" width="400" />
</p>

A **rack** is the loop of gear slung over a climber's shoulder — cams, nuts, draws, everything needed for the route ahead. `go-rack` carries your coding-agent tools the same way: a lightweight Go library that keeps multiple AI providers organized behind a single, common interface.

Current providers:
- Claude CLI (`go-rack/claude`)
- Codex CLI (`go-rack/codex`)

## Install

```bash
go get go-rack
```

You also need the provider CLIs installed and authenticated:
- `claude` for Claude provider
- `codex` for Codex provider

## Core Interface

Both providers implement `rack.Agent`:

```go
type Agent interface {
    Run(ctx context.Context, prompt string, opts ...RunOption) (Result, error)
}
```

This lets you swap providers without changing your calling code.

## Claude Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    rack "go-rack"
    "go-rack/claude"
)

func main() {
    ctx := context.Background()

    client := claude.NewClient(
        claude.WithDefaultModel("sonnet"),
    )

    res, err := client.Run(
        ctx,
        "Write a small Go function that reverses a string.",
        rack.WithMaxTurns(3),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(res.Text)
}
```

## Codex Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    rack "go-rack"
    "go-rack/codex"
)

func main() {
    ctx := context.Background()

    client := codex.NewClient(
        codex.WithDefaultModel("gpt-5-codex"),
    )

    res, err := client.Run(
        ctx,
        "Refactor this function for readability and keep behavior identical.",
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(res.Text)
}
```

## Shared Run Options

These `rack` options work across providers:
- `rack.WithModel(...)`
- `rack.WithSystemPrompt(...)`
- `rack.WithEventHandler(...)`
- `rack.WithOutputStream(...)`
- `rack.WithTraceID(...)`

## Provider Differences

Claude supports additional controls:
- `rack.WithMaxTurns(...)`
- `rack.WithMaxOutputTokens(...)`
- `rack.WithAllowedTools(...)`
- `rack.WithDisallowedTools(...)`

Codex currently returns explicit errors if those four options are set.

## Streaming Events

Use `rack.WithEventHandler` to receive normalized events (`assistant`, `assistant_start`, `tool_use`, `tool_result`, `result`, `result_error`):

```go
handler := func(ev rack.Event) {
    switch ev.Type {
    case rack.EventAssistant:
        fmt.Print(ev.Text)
    case rack.EventResultError:
        fmt.Println("run failed:", ev.Text)
    }
}
```

Then pass it to `Run`:

```go
res, err := client.Run(ctx, prompt, rack.WithEventHandler(handler))
```

## Slack Notifications

The `go-rack/slack` package provides shared Slack notifications for any agent — zero external dependencies, raw HTTP only. It supports both webhook and bot-token modes, with automatic threading and `@mention` on session end.

### Config

Embed `slack.Config` in your agent's config:

```yaml
slack:
  enabled: true
  bot_token: "xoxb-your-bot-token"
  channel: "C0123456789"
  notify_users:
    - "U0123456789"
  events:
    on_start: true
    on_error: true
    on_result: true
    on_tool_use: false
```

Webhook-only mode (no threading) is also supported — set `webhook_url` instead of `bot_token`/`channel`.

### Explicit Notifier API

Thread-aware, concurrency-safe. All methods no-op when disabled — no nil checks needed.

```go
import rackslack "go-rack/slack"

notifier := rackslack.NewNotifier(cfg.Slack)
notifier.StartSession(ctx, "Session started")   // captures thread TS
notifier.Send(ctx, "Working on todo #1...")      // threads under session
notifier.EndSession(ctx, "All done!")            // threads + @mentions
```

### Auto EventHandler

Returns a `rack.EventHandler` that dispatches Slack notifications based on `EventConfig`. Calls are made in goroutines so the handler never blocks the event stream.

```go
handler := rackslack.NewEventHandler(notifier,
    rackslack.WithHandlerAgentName("ralph"),
)
res, err := client.Run(ctx, prompt, rack.WithEventHandler(handler))
```

Options: `WithHandlerAgentName`, `WithErrorFormatter`, `WithResultFormatter`, `WithHandlerContext`.

### Composing with Logger

```go
slackH := rackslack.NewEventHandler(notifier)
logH := rack.NewLogger(os.Stderr)
combined := func(e rack.Event) { logH(e); slackH(e) }
res, err := client.Run(ctx, prompt, rack.WithEventHandler(combined))
```

## Observability

Both providers support pluggable observability via:
- `claude.WithObservability(...)`
- `codex.WithObservability(...)`

Use `rack.WithTraceID(...)` on each run to attach completions to a trace.
