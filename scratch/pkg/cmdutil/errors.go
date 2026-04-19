package cmdutil

import (
	"errors"
	"fmt"
)

//定义三种不同error类型的原因
//SilentError表示命令已经自己打印了错误，主循环只需退出，不需要重复打印
//CancelError表示用户自己取消
//FlagError携带完整错误信息，触发usage打印，包括命令错误或者help等

// SilentError is an error that signals the CLI should exit
// but should not print an error message
var SilentError = errors.New("silent error")

// CancelError signals that the user cancelled an interactive prompt
var CancelError = errors.New("cancel error")

// FlagError wraps errors that originateed from flag usage
type FlagError struct {
	Err error
}

func (f *FlagError) Error() string {
	return f.Err.Error()
}
func (f *FlagError) Unwrap() error { return f.Err }

// NewFlagErrorf creates a FlagError with a formatted message
func NewFlagErrorf(format string, args ...any) *FlagError {
	return &FlagError{fmt.Errorf(format, args...)}
}

// IsUserCancellation reports whether err represents a user cancellation.
func IsUserCancellation(err error) bool {
	return errors.Is(err, CancelError)
}
