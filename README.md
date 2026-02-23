# belaykit

<p align="center">
  <img src="belaykit.png" alt="belaykit" width="400" />
</p>

**A rope for your agent harness.**

In climbing, a belay provides a climber protection from falling through a rope and a [device](https://github.com/hev/belaydevice), collectively referred to as a belay kit. Belaykit allows you to "belay" your agentic coding harness by centralizing common concerns like observability, failover, and interrupt behavior into a centralized Go library.

## Providers

- Claude CLI (`belaykit/claude`)
- Codex CLI (`belaykit/codex`)

## Install

```bash
go get belaykit
```

You also need the provider CLIs installed and authenticated:
- `claude` for Claude provider
- `codex` for Codex provider

## Usage

Both providers implement `belaykit.Agent`:

```go
type Agent interface {
    Run(ctx context.Context, prompt string, opts ...RunOption) (Result, error)
}
```

### Claude

```go
client := claude.NewClient(
    claude.WithDefaultModel("sonnet"),
)

res, err := client.Run(
    ctx,
    "Write a small Go function that reverses a string.",
    belaykit.WithMaxTurns(3),
)
```

### Codex

```go
client := codex.NewClient(
    codex.WithDefaultModel("gpt-5-codex"),
)

res, err := client.Run(
    ctx,
    "Refactor this function for readability and keep behavior identical.",
)
```

## Run Options

Shared options that work across providers:
- `belaykit.WithModel(...)`
- `belaykit.WithSystemPrompt(...)`
- `belaykit.WithEventHandler(...)`
- `belaykit.WithOutputStream(...)`
- `belaykit.WithTraceID(...)`

Claude-specific:
- `belaykit.WithMaxTurns(...)`
- `belaykit.WithMaxOutputTokens(...)`
- `belaykit.WithAllowedTools(...)`
- `belaykit.WithDisallowedTools(...)`

## Streaming Events

```go
handler := func(ev belaykit.Event) {
    switch ev.Type {
    case belaykit.EventAssistant:
        fmt.Print(ev.Text)
    case belaykit.EventResultError:
        fmt.Println("run failed:", ev.Text)
    }
}

res, err := client.Run(ctx, prompt, belaykit.WithEventHandler(handler))
```

## Slack Notifications

The `belaykit/slack` package sends Slack notifications for any agent using raw HTTP (no external dependencies). Supports webhook and bot-token modes with automatic threading.

```go
import rackslack "belaykit/slack"

notifier := rackslack.NewNotifier(cfg.Slack)
notifier.StartSession(ctx, "Session started")
notifier.Send(ctx, "Working on todo #1...")
notifier.EndSession(ctx, "All done!")
```

Or use the auto event handler:

```go
handler := rackslack.NewEventHandler(notifier,
    rackslack.WithHandlerAgentName("ralph"),
)
res, err := client.Run(ctx, prompt, belaykit.WithEventHandler(handler))
```

## Observability

Both providers support pluggable observability:
- `claude.WithObservability(...)`
- `codex.WithObservability(...)`

Use `belaykit.WithTraceID(...)` on each run to attach completions to a trace.

Pair with [belaydevice](https://github.com/hev/belaydevice) to visualize agent trace trees — phases, tool calls, token usage, cost, and context window utilization.
