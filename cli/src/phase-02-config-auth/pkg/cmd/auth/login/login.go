// Package login provides the `gh auth login` command.
package login

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/learngh/gh-impl/internal/authflow"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// LoginOptions holds the dependencies and configuration for `gh auth login`.
type LoginOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (cmdutil.Config, error)
	HttpClient func() (*http.Client, error)
	Hostname   string
	Token      string // populated from stdin when WithToken is true
	WithToken  bool
}

// NewCmdLogin creates the `gh auth login` cobra command.
func NewCmdLogin(f *cmdutil.Factory) *cobra.Command {
	opts := &LoginOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a GitHub host",
		Long: `Authenticate with a GitHub host.

The default authentication mode is an interactive OAuth device flow.
Alternatively, pass in a token on standard input by using --with-token.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.WithToken {
				reader := bufio.NewReader(opts.IO.In)
				tok, err := reader.ReadString('\n')
				if err != nil && !errors.Is(err, io.EOF) {
					return fmt.Errorf("reading token from stdin: %w", err)
				}
				opts.Token = strings.TrimSpace(tok)
				if opts.Token == "" {
					return fmt.Errorf("--with-token: no token provided on stdin")
				}
			}
			return loginRun(opts)
		},
	}

	// Pre-register --help without the -h shorthand: login uses -h for --hostname,
	// so we claim the "help" name first to prevent cobra from trying to bind -h to it.
	cmd.Flags().Bool("help", false, "Show help for login")
	cmd.Flags().Lookup("help").Hidden = true

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "github.com", "The hostname of the GitHub instance to authenticate with")
	cmd.Flags().BoolVar(&opts.WithToken, "with-token", false, "Read token from standard input")

	return cmd
}

// loginRun performs the actual login logic.
func loginRun(opts *LoginOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	if opts.WithToken {
		apiBase := apiBaseURL(opts.Hostname)
		username, err := authflow.FetchUsername(httpClient, apiBase, opts.Token)
		if err != nil {
			return fmt.Errorf("authenticating with token: %w", err)
		}
		if err := cfg.Login(opts.Hostname, username, opts.Token); err != nil {
			return fmt.Errorf("saving credentials: %w", err)
		}
		fmt.Fprintf(opts.IO.Out, "Logged in to %s as %s\n", opts.Hostname, username)
		return nil
	}

	// Interactive OAuth Device Flow.
	// DeviceFlow prints "Logged in as <user>" internally; loginRun does not repeat it.
	result, err := authflow.DeviceFlow(httpClient, opts.Hostname, opts.IO)
	if err != nil {
		return err
	}
	if err := cfg.Login(opts.Hostname, result.Username, result.Token); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}
	return nil
}

// apiBaseURL returns the GitHub API base URL for the given hostname.
func apiBaseURL(hostname string) string {
	if hostname == "github.com" {
		return "https://api.github.com"
	}
	return "https://" + hostname + "/api/v3"
}
