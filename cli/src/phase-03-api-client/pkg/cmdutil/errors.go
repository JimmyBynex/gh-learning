package cmdutil

import (
	"errors"
	"fmt"
)

// SilentError is an error that signals the CLI should exit with an error code
// but should not print an error message (the command already printed one).
var SilentError = errors.New("silent error")

// CancelError signals that the user cancelled an interactive prompt.
var CancelError = errors.New("cancel error")

// FlagError wraps errors that originate from invalid flag usage.
type FlagError struct {
	Err error
}

func (f *FlagError) Error() string {
	return f.Err.Error()
}

func (f *FlagError) Unwrap() error {
	return f.Err
}

// NewFlagErrorf creates a FlagError with a formatted message.
func NewFlagErrorf(format string, args ...any) *FlagError {
	return &FlagError{Err: fmt.Errorf(format, args...)}
}

// IsUserCancellation reports whether err represents a user cancellation.
func IsUserCancellation(err error) bool {
	return errors.Is(err, CancelError)
}
