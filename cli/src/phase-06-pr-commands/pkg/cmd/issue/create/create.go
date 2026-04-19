package create

import (
	"fmt"
	"net/http"

	"github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/internal/ghrepo"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// CreateOptions holds all inputs for the issue create command.
type CreateOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Repo       string
	Title      string
	Body       string
}

// NewCmdIssueCreate creates the `gh issue create` command.
func NewCmdIssueCreate(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			return createRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Supply a title (required)")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Supply a body")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func createRun(opts *CreateOptions) error {
	if opts.Repo == "" {
		return fmt.Errorf("repository required: use --repo owner/name")
	}
	repo, err := ghrepo.FromFullName(opts.Repo)
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	params := map[string]interface{}{
		"title": opts.Title,
		"body":  opts.Body,
	}
	issue, err := api.CreateIssue(client, repo, params)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.Out, "Created issue #%d: %s\n%s\n", issue.Number, issue.Title, issue.URL)
	return nil
}
