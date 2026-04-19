package iostreams

import (
	"bytes"
	"io"
	"os"

	"golang.org/x/term"
)

// IOStreams holds the standard input/output/error streams used throughout the CLI.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	// TTY state (override mechanism)
	stdinTTYOverride  bool
	stdinIsTTY        bool
	stdoutTTYOverride bool
	stdoutIsTTY       bool

	colorEnabled bool

	neverPrompt bool
}

// ColorEnabled reports whether color output is enabled.
func (s *IOStreams) ColorEnabled() bool {
	return s.colorEnabled
}

// CanPrompt reports whether interactive prompts are allowed.
func (s *IOStreams) CanPrompt() bool {
	if s.neverPrompt {
		return false
	}
	return s.IsStdinTTY() && s.IsStdoutTTY()
}

// IsStdinTTY reports whether standard input is a terminal.
func (s *IOStreams) IsStdinTTY() bool {
	if s.stdinTTYOverride {
		return s.stdinIsTTY
	}
	f, ok := s.In.(*os.File)
	return ok && isTerminal(f)
}

// IsStdoutTTY reports whether standard output is a terminal.
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

// SetColorEnabled sets whether color output is enabled.
func (s *IOStreams) SetColorEnabled(v bool) {
	s.colorEnabled = v
}

// SetNeverPrompt sets whether interactive prompts are suppressed.
func (s *IOStreams) SetNeverPrompt(v bool) {
	s.neverPrompt = v
}

// System returns an IOStreams connected to the real stdin/stdout/stderr.
func System() *IOStreams {
	return &IOStreams{
		In:           os.Stdin,
		Out:          os.Stdout,
		ErrOut:       os.Stderr,
		colorEnabled: isTerminal(os.Stdout),
	}
}

// Test returns an IOStreams backed by byte buffers for use in tests.
// It returns the IOStreams and in/out/errOut buffers respectively.
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	ios := &IOStreams{
		In:     in,
		Out:    out,
		ErrOut: errOut,
	}
	ios.SetStdinTTY(false)
	ios.SetStdoutTTY(false)
	return ios, in, out, errOut
}

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
