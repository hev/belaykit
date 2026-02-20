package rack

import "context"

// Agent is the interface that all coding agent backends implement.
type Agent interface {
	Run(ctx context.Context, prompt string, opts ...RunOption) (Result, error)
}

// Result holds the response from an agent invocation.
type Result struct {
	Text string
}
