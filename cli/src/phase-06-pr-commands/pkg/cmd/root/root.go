package root

import (
	"fmt"
	"io"

	"github.com/learngh/gh-impl/pkg/cmd/auth"
	apiCmd "github.com/learngh/gh-impl/pkg/cmd/api"
	issueCmd "github.com/learngh/gh-impl/pkg/cmd/issue"
	prCmd "github.com/learngh/gh-impl/pkg/cmd/pr"
	repoCmd "github.com/learngh/gh-impl/pkg/cmd/repo"
	"github.com/learngh/gh-impl/pkg/cmd/version"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// AuthError wraps an authentication error so callers can distinguish it from
// generic errors and exit with the auth exit code.
type AuthError struct {
	err error
}

func (ae *AuthError) Error() string {
	return ae.err.Error()
}

// NewAuthError creates an AuthError wrapping the given error.
func NewAuthError(err error) *AuthError {
	return &AuthError{err: err}
}

// RootOptions holds the dependencies for the root command's run logic.
type RootOptions struct {
	Out         io.Writer
	VersionInfo string
	ShowVersion func() bool
	ShowHelp    func() error
}

// NewCmdRoot builds and returns the root cobra command.
func NewCmdRoot(f *cmdutil.Factory, ver string) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "gh <command> <subcommand> [flags]",
		Short: "GitHub CLI",
		Long:  "GitHub CLI\n\nWork seamlessly with GitHub from the command line.",
		Annotations: map[string]string{
			"versionInfo": version.Format(ver),
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Phase 2 will add auth checks here.
			return nil
		},
	}

	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	// --help is registered explicitly as a persistent flag so subcommands can
	// look it up via PersistentFlags(). Cobra also adds its own help flag, but
	// the explicit registration here makes the flag available on the persistent set.
	cmd.PersistentFlags().Bool("help", false, "Show help for command")

	// --version flag on the root command.
	cmd.Flags().Bool("version", false, "Show gh version")

	opts := &RootOptions{
		Out:         f.IOStreams.Out,
		VersionInfo: version.Format(ver),
		ShowVersion: func() bool {
			v, _ := cmd.Flags().GetBool("version")
			return v
		},
		ShowHelp: cmd.Help,
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return rootRun(opts)
	}

	// Hidden "version" sub-command for `gh version`.
	cmd.AddCommand(version.NewCmdVersion(f, ver))

	// Auth command group.
	cmd.AddCommand(auth.NewCmdAuth(f))

	// API command.
	cmd.AddCommand(apiCmd.NewCmdAPI(f))

	// Repo command group.
	cmd.AddCommand(repoCmd.NewCmdRepo(f))

	// Issue command group.
	cmd.AddCommand(issueCmd.NewCmdIssue(f))

	// PR command group.
	cmd.AddCommand(prCmd.NewCmdPR(f))

	return cmd, nil
}

func rootRun(opts *RootOptions) error {
	if opts.ShowVersion() {
		fmt.Fprint(opts.Out, opts.VersionInfo)
		return nil
	}
	return opts.ShowHelp()
}
