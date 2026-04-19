package list

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/internal/ghrepo"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ListOptions holds all inputs for the pr list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Repo       string
	State      string
	Limit      int
}

// NewCmdPRList creates the `gh pr list` command.
func NewCmdPRList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		State:      "open",
		Limit:      30,
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pull requests in a repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVarP(&opts.State, "state", "s", "open", "Filter by state: open, closed, merged, all")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of pull requests to fetch")
	return cmd
}

func listRun(opts *ListOptions) error {
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

	var graphqlState string
	switch strings.ToLower(opts.State) {
	case "open":
		graphqlState = "OPEN"
	case "closed":
		graphqlState = "CLOSED"
	case "merged":
		graphqlState = "MERGED"
	case "all":
		graphqlState = ""
	default:
		return fmt.Errorf("invalid state %q: use open, closed, merged, or all", opts.State)
	}

	var prs []api.PullRequest
	if graphqlState == "" {
		for _, s := range []string{"OPEN", "CLOSED", "MERGED"} {
			got, err := api.ListPullRequests(client, repo, s, opts.Limit)
			if err != nil {
				return err
			}
			prs = append(prs, got...)
		}
	} else {
		prs, err = api.ListPullRequests(client, repo, graphqlState, opts.Limit)
		if err != nil {
			return err
		}
	}

	if len(prs) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "No pull requests found.\n")
		return nil
	}
	for _, pr := range prs {
		draft := ""
		if pr.IsDraft {
			draft = " [DRAFT]"
		}
		fmt.Fprintf(opts.IO.Out, "#%-5d  %s%s\t(%s -> %s)\n",
			pr.Number, pr.Title, draft, pr.HeadRefName, pr.BaseRefName)
	}
	return nil
}
