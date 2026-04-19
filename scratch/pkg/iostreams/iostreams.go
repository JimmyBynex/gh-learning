package iostreams

import (
	"bytes"
	"io"
	"os"

	"golang.org/x/term"
)

// IOStreams持有全部CLI进出的数据
type IOStreams struct {
	//三个是主要使用的
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	//下面判别TTY,前面两个是否手动修改，后面两个是判断值
	//这是用户意愿，优先级更高
	//强制取消或者改变TTY,也是更加方便测试，因为实际测试的时候没有TTY，用类型和内核判断无法实现
	stdinTTYOverride  bool
	stdoutTTYOverride bool
	stdinIsTTY        bool
	stdoutIsTTY       bool

	colorEnabled bool

	//这是服务于自动化的，具体交互与否
	neverPrompt bool
}

// ColorEnabled reports whether color output is enabled
func (s *IOStreams) ColorEnabled() bool {
	return s.colorEnabled
}

// CanPrompt reports whether interactive prompts are allowed
func (s *IOStreams) CanPrompt() bool {
	if s.neverPrompt {
		return false
	}
	//这里再确认一次是因为防止管道拼接，或者说这里才是实际真实判断

	//return s.stdoutIsTTY && s.stdinIsTTY
	//这是我一开始的错误写法，实际上再先判断是否能够prompt之后，不能直接看是不是TTY，这是我手动规定的，需要走下面两个函数按三层优先级判断应该不应该判断是TTY
	return s.IsStdoutTTY() && s.IsStdinTTY()
}

// IsStdinTTY reports whether standard inputs is a terminal
func (s *IOStreams) IsStdinTTY() bool {
	//相当于最高优先级是用户
	if s.stdinTTYOverride {
		return s.stdinIsTTY
	}
	//次高优先级是类型判断
	f, ok := s.In.(*os.File)
	//最后才是内核判断
	return ok && isTerminal(f)
}

// IsStdoutTTY reports whether standard outputs is a terminal
func (s *IOStreams) IsStdoutTTY() bool {
	if s.stdoutTTYOverride {
		return s.stdoutIsTTY
	}
	f, ok := s.Out.(*os.File)
	return ok && isTerminal(f)
}

// SetStdinTTY overrides whether stdin is treated as a terminal.
func (s *IOStreams) SetStdinTTY(v bool) {
	s.stdinTTYOverride = true
	s.stdinIsTTY = v
}

// SetStdoutTTY overrides whether stdout is treated as a terminal.
func (s *IOStreams) SetStdoutTTY(v bool) {
	s.stdoutTTYOverride = true
	s.stdoutIsTTY = v
}

// SetColorEnabled sets whether color output is enabled
func (s *IOStreams) SetColorEnabled(v bool) {
	s.colorEnabled = v
}

// SetNeverPrompt sets whether interactive prompt is suppressed
func (s *IOStreams) SetNeverPrompt(v bool) {
	s.neverPrompt = v
}

// System returns an IOStreams connected to the real stdin/stdout/stderr
func System() *IOStreams {
	return &IOStreams{
		In:           os.Stdin,
		Out:          os.Stdout,
		ErrOut:       os.Stderr,
		colorEnabled: isTerminal(os.Stdout),
	}
}

// isTerminal reports whether f is a terminal file
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// Test returns an IOStreams backed by byte buffers for use in tests
// It returns the IOStreams and in/out/errout buffers respectively
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	err := &bytes.Buffer{}
	ios := &IOStreams{
		In:     in,
		Out:    out,
		ErrOut: err,
	}
	//为什么在实际修改成buffer之后还要手动修改逻辑层？
	//gemini回答
	// 手动 Override 机制将“物理环境”与“程序逻辑”彻底解耦，赋予了测试在任何环境下都能“无中生有”模拟终端或“绝对静默”屏蔽干扰的能力，从而确保了 CLI 框架的确定性与高度可测试性
	ios.SetStdinTTY(false)
	ios.SetStdoutTTY(false)
	return ios, in, out, err
}
