package status

import (
	"fmt"
	"net/http"
	"scratch/pkg/cmdutil"
	"scratch/pkg/iostreams"

	"github.com/spf13/cobra"
)

// StatusOptions holds the dependencies and configuration for 'gh auth status'
type StatusOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (cmdutil.Config, error)
	httpClient func() (*http.Client, error)
}

// NewCmdStatus creates the 'gh auth status' cobra command
func NewCmdStatus(f *cmdutil.Factory) *cobra.Command {
	opts := &StatusOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		httpClient: f.HttpClient,
	}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "View authentication status",
		Long:  "Verifies and displays information about your authentication state.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return statusRun(opts)
		},
	}
	return cmd
}

// statusRun displays the authentication status for all configured hostnames.
func statusRun(opts *StatusOptions) error {
	//先是懒加载
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	hosts := cfg.Hosts()
	if len(hosts) == 0 {
		//提醒当前需要重新登录
		fmt.Fprintf(opts.IO.ErrOut, "You are not logged in to any GitHub hosts. Run `gh auth login` to authenticate.\n")
		return cmdutil.SilentError
	}
	//再检查当前全部host
	for _, hostname := range hosts {
		token, err := cfg.AuthToken(hostname)
		if err != nil || token == "" {
			fmt.Fprintf(opts.IO.Out, "%s\n  x Not logged in\n\n", hostname)
			continue
		}
		username, err := cfg.Get(hostname, "user")
		if err != nil {
			return err
		}

		fmt.Fprintf(opts.IO.Out, "%s\n  Logged in to %s as %s\n\n", hostname, hostname, username)
	}
	return nil
}
