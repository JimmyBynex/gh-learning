package root

import (
	"fmt"
	"io"
	"scratch/pkg/cmd/api"
	"scratch/pkg/cmd/auth"
	"scratch/pkg/cmd/repo"
	"scratch/pkg/cmd/version"
	"scratch/pkg/cmdutil"

	"github.com/spf13/cobra"
)

// RootOptions holds the dependencies for the root command's run logic
// 相当于一个闭包捕获当前cmd，ver和stdout
type RootOptions struct {
	Out         io.Writer
	VersionInfo string
	ShowVersion func() bool
	ShowHelp    func() error
}

// AuthError wraps an authentication error so callers can distinguish it from generic errors and exit with the auth exit code
// 隐式实现
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

// NewCmdRoot builds and returns the root cobra command
func NewCmdRoot(f *cmdutil.Factory, ver string) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "gh <command> <subcommand> [flags]",
		Short: "GitHub CLI",
		Long:  "GitHub CLI\n\nWork seamlessly with GitHub from command line.",
		Annotations: map[string]string{
			"VersionInfo": version.Format(ver),
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	//在cobra包中，--help触发errHelp走分支，是不会使用业务代码中的（run/runE）
	//PersisitentFlags意味着当前命令和子命令都注册这个flag
	cmd.PersistentFlags().BoolP("help", "h", false, "Show help for command")
	cmd.Flags().BoolP("version", "v", false, "Show gh version")

	opts := &RootOptions{
		Out:         f.IOStreams.Out,
		VersionInfo: ver,
		ShowHelp:    cmd.Help,
		ShowVersion: func() bool {
			v, _ := cmd.Flags().GetBool("version")
			return v
		},
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return rootRun(opts)
	}

	//添加子命令
	cmd.AddCommand(version.NewCmdVersion(f, ver))
	cmd.AddCommand(auth.NewCmdAuth(f))
	cmd.AddCommand(api.NewCmdApi(f))
	cmd.AddCommand(repo.NewCmdRepo(f))
	return cmd, nil

}

func rootRun(opts *RootOptions) error {
	if opts.ShowVersion() {
		fmt.Fprintf(opts.Out, opts.VersionInfo)
		return nil
	}
	return opts.ShowHelp()
}
