package claude

import (
	"errors"
	"fmt"
)

// ErrNoJSON indicates no JSON object or array was found in the response.
var ErrNoJSON = errors.New("no JSON found in response")

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
