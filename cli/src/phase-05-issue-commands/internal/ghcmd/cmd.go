package ghcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/learngh/gh-impl/internal/build"
	"github.com/learngh/gh-impl/internal/factory"
	"github.com/learngh/gh-impl/pkg/cmd/root"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type exitCode int

const (
	exitOK     exitCode = 0
	exitError  exitCode = 1
	exitCancel exitCode = 2
	exitAuth   exitCode = 4
)

// Main is the primary entry point for the gh CLI. It returns an exit code that
// the caller (main.go) passes directly to os.Exit.
func Main() exitCode {
	buildVersion := build.Version
	cmdFactory := factory.New(buildVersion)
	stderr := cmdFactory.IOStreams.ErrOut

	rootCmd, err := root.NewCmdRoot(cmdFactory, buildVersion)
	if err != nil {
		fmt.Fprintf(stderr, "failed to create root command: %s\n", err)
		return exitError
	}

	rootCmd.SetArgs(os.Args[1:])

	if cmd, err := rootCmd.ExecuteC(); err != nil {
		var authError *root.AuthError
		if errors.Is(err, cmdutil.SilentError) {
			return exitError
		} else if cmdutil.IsUserCancellation(err) {
			return exitCancel
		} else if errors.As(err, &authError) {
			return exitAuth
		}
		printError(stderr, err, cmd)
		return exitError
	}
	return exitOK
}

// printError writes err to out. When the error looks like an unknown command or
// a flag error, the command's usage string is also printed.
func printError(out io.Writer, err error, cmd *cobra.Command) {
	fmt.Fprintln(out, err)

	var flagError *cmdutil.FlagError
	// strings.HasPrefix on cobra's error message is coupled to cobra internals;
	// cobra does not expose a typed "unknown command" error.
	if errors.As(err, &flagError) || strings.HasPrefix(err.Error(), "unknown command ") {
		if !strings.HasSuffix(err.Error(), "\n") {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, cmd.UsageString())
	}
}
