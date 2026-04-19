// Package status provides the `gh auth status` command.
package status

import (
	"fmt"
	"net/http"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// StatusOptions holds the dependencies and configuration for `gh auth status`.
type StatusOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (cmdutil.Config, error)
	HttpClient func() (*http.Client, error)
}

// NewCmdStatus creates the `gh auth status` cobra command.
func NewCmdStatus(f *cmdutil.Factory) *cobra.Command {
	opts := &StatusOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	return &cobra.Command{
		Use:   "status",
		Short: "View authentication status",
		Long:  "Verifies and displays information about your authentication state.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return statusRun(opts)
		},
	}
}

// statusRun displays the authentication status for all configured hostnames.
func statusRun(opts *StatusOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	hosts := cfg.Hosts()
	if len(hosts) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "You are not logged in to any GitHub hosts. Run `gh auth login` to authenticate.\n")
		return cmdutil.SilentError
	}

	for _, hostname := range hosts {
		token, err := cfg.AuthToken(hostname)
		if err != nil || token == "" {
			fmt.Fprintf(opts.IO.Out, "%s\n  x Not logged in\n\n", hostname)
			continue
		}

		username, _ := cfg.Get(hostname, "user")
		if username == "" {
			username = "(unknown)"
		}

		fmt.Fprintf(opts.IO.Out, "%s\n  Logged in to %s as %s\n\n", hostname, hostname, username)
	}

	return nil
}
