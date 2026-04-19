// 文件：pkg/cmd/repo/list/list.go
package list

import (
	"fmt"
	"net/http"
	"scratch/api"
	"scratch/pkg/cmdutil"
	"scratch/pkg/iostreams"

	"github.com/spf13/cobra"
)

// ListOptions holds all inputs for the repo list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Config     func() (cmdutil.Config, error)
	Login      string // GitHub username; if empty, use authenticated user
	Limit      int
}

// NewCmdRepoList creates the `gh repo list` command.
func NewCmdRepoList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Limit:      30,
	}

	cmd := &cobra.Command{
		Use:   "list [<user>]",
		Short: "List repositories",
		Long:  "List GitHub repositories.\n\nWith no argument, lists repositories for the authenticated user.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Login = args[0]
			}
			return listRun(opts)
		},
	}
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of repositories to list")
	return cmd
}

func listRun(opts *ListOptions) error {
	login := opts.Login
	if login == "" {
		// Use the authenticated user's login from config.
		cfg, err := opts.Config()
		if err != nil {
			return fmt.Errorf("could not load config: %w", err)
		}
		tok, err := cfg.AuthToken("github.com")
		if err != nil || tok == "" {
			return fmt.Errorf("not authenticated: run `gh auth login`")
		}
		// Fetch the current user's login via the API.
		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}
		client := api.NewClientFormHTTP(httpClient)
		login, err = fetchCurrentUser(client)
		if err != nil {
			return err
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFormHTTP(httpClient)

	repos, err := api.ListRepositories(client, login, opts.Limit)
	if err != nil {
		return err
	}

	for _, r := range repos {
		visibility := "public"
		if r.IsPrivate {
			visibility = "private"
		}
		fmt.Fprintf(opts.IO.Out, "%-40s\t%s\n", r.NameWithOwner, visibility)
	}
	return nil
}

// fetchCurrentUser returns the authenticated user's login name.
func fetchCurrentUser(client *api.Client) (string, error) {
	var result struct {
		Login string `json:"login"`
	}
	if err := client.REST("github.com", "GET", "user", nil, &result); err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}
	return result.Login, nil
}
