# go-rack

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

## Observability

Both providers support pluggable observability via:
- `claude.WithObservability(...)`
- `codex.WithObservability(...)`

Use `rack.WithTraceID(...)` on each run to attach completions to a trace.
