package iostreams_test

import (
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

func TestTest_returnsBuffers(t *testing.T) {
	ios, in, out, errOut := iostreams.Test()
	if ios == nil {
		t.Fatal("expected non-nil IOStreams")
	}
	if in == nil || out == nil || errOut == nil {
		t.Fatal("expected non-nil buffers")
	}
}

func TestTest_isNotTTY(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if ios.IsStdinTTY() {
		t.Error("expected stdin to not be a TTY in test mode")
	}
	if ios.IsStdoutTTY() {
		t.Error("expected stdout to not be a TTY in test mode")
	}
}

func TestTest_canPrompt_false(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if ios.CanPrompt() {
		t.Error("expected CanPrompt() to return false in test mode")
	}
}

func TestSetNeverPrompt(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	// Even if TTY were overridden to true, NeverPrompt should win.
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	ios.SetNeverPrompt(true)
	if ios.CanPrompt() {
		t.Error("expected CanPrompt() false when NeverPrompt is set")
	}
}

func TestSetStdinTTY_override(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	if !ios.IsStdinTTY() {
		t.Error("expected IsStdinTTY() true after override")
	}
	ios.SetStdinTTY(false)
	if ios.IsStdinTTY() {
		t.Error("expected IsStdinTTY() false after override set to false")
	}
}

func TestSetStdoutTTY_override(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	if !ios.IsStdoutTTY() {
		t.Error("expected IsStdoutTTY() true after override")
	}
}

func TestColorEnabled(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if ios.ColorEnabled() {
		t.Error("expected color disabled by default in test mode")
	}
	ios.SetColorEnabled(true)
	if !ios.ColorEnabled() {
		t.Error("expected color enabled after SetColorEnabled(true)")
	}
}

func TestSystem_fields(t *testing.T) {
	ios := iostreams.System()
	if ios.In == nil {
		t.Error("expected non-nil In")
	}
	if ios.Out == nil {
		t.Error("expected non-nil Out")
	}
	if ios.ErrOut == nil {
		t.Error("expected non-nil ErrOut")
	}
}

func TestIOStreams_readWrite(t *testing.T) {
	ios, in, out, errOut := iostreams.Test()

	// Write to in buffer and read via ios.In.
	in.WriteString("hello")
	buf := make([]byte, 5)
	n, err := ios.In.Read(buf)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("got %q, want %q", string(buf[:n]), "hello")
	}

	// Write to ios.Out.
	ios.Out.Write([]byte("world"))
	if out.String() != "world" {
		t.Errorf("got %q, want %q", out.String(), "world")
	}

	// Write to ios.ErrOut.
	ios.ErrOut.Write([]byte("error!"))
	if !strings.Contains(errOut.String(), "error!") {
		t.Errorf("errOut did not contain expected string")
	}
}
