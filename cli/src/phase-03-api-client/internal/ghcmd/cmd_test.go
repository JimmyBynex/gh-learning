package ghcmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// newTestCmd returns a minimal cobra.Command for use in printError tests.
func newTestCmd(use string) *cobra.Command {
	return &cobra.Command{Use: use}
}

func TestPrintError_plainError(t *testing.T) {
	var buf bytes.Buffer
	cmd := newTestCmd("gh")

	printError(&buf, errors.New("something went wrong"), cmd)

	got := buf.String()
	if !strings.Contains(got, "something went wrong") {
		t.Errorf("output %q does not contain error message", got)
	}
	// plain error: usage should NOT be printed
	if strings.Contains(got, "Usage:") {
		t.Errorf("plain error should not print usage, got: %q", got)
	}
}

func TestPrintError_flagError_printsUsage(t *testing.T) {
	var buf bytes.Buffer
	cmd := newTestCmd("gh")
	// cobra's UsageString requires a non-empty Use field; add a flag so usage has content.
	cmd.Flags().Bool("dry-run", false, "dry run")

	flagErr := cmdutil.NewFlagErrorf("unknown flag: --bogus")
	printError(&buf, flagErr, cmd)

	got := buf.String()
	if !strings.Contains(got, "unknown flag: --bogus") {
		t.Errorf("output %q does not contain flag error message", got)
	}
	if !strings.Contains(got, "Usage:") {
		t.Errorf("FlagError should print usage, got: %q", got)
	}
}

func TestPrintError_unknownCommand_printsUsage(t *testing.T) {
	var buf bytes.Buffer
	cmd := newTestCmd("gh")

	printError(&buf, errors.New("unknown command \"foobar\" for \"gh\""), cmd)

	got := buf.String()
	if !strings.Contains(got, "unknown command") {
		t.Errorf("output %q does not contain 'unknown command'", got)
	}
	if !strings.Contains(got, "Usage:") {
		t.Errorf("unknown command error should print usage, got: %q", got)
	}
}

func TestPrintError_flagError_noTrailingNewline(t *testing.T) {
	// When the error message already ends with '\n', printError must not add an
	// extra blank line before the usage string.
	var buf bytes.Buffer
	cmd := newTestCmd("gh")

	flagErr := cmdutil.NewFlagErrorf("flag error with newline\n")
	printError(&buf, flagErr, cmd)

	got := buf.String()
	// Should not have a double blank line between the error and usage.
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("unexpected triple newline in output: %q", got)
	}
}
