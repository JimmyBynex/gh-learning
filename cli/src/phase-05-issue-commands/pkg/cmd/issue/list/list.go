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

// ListOptions holds all inputs for the issue list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Repo       string // --repo flag: owner/name
	State      string // --state flag: open|closed|all
	Limit      int    // --limit flag
}

// NewCmdIssueList creates the `gh issue list` command.
func NewCmdIssueList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		State:      "open",
		Limit:      30,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues in a repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVarP(&opts.State, "state", "s", "open", "Filter by state: open, closed, all")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of issues to fetch")
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

	// Map user-facing state to GraphQL IssueState enum.
	var graphqlState string
	switch strings.ToLower(opts.State) {
	case "open":
		graphqlState = "OPEN"
	case "closed":
		graphqlState = "CLOSED"
	case "all":
		graphqlState = "" // handled below
	default:
		return fmt.Errorf("invalid state %q: use open, closed, or all", opts.State)
	}

	var issues []api.Issue
	if graphqlState == "" {
		// Fetch both open and closed.
		open, err := api.ListIssues(client, repo, "OPEN", opts.Limit)
		if err != nil {
			return err
		}
		closed, err := api.ListIssues(client, repo, "CLOSED", opts.Limit)
		if err != nil {
			return err
		}
		issues = append(open, closed...)
	} else {
		issues, err = api.ListIssues(client, repo, graphqlState, opts.Limit)
		if err != nil {
			return err
		}
	}

	if len(issues) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "No issues found.\n")
		return nil
	}
	for _, issue := range issues {
		fmt.Fprintf(opts.IO.Out, "#%-5d  %s\n", issue.Number, issue.Title)
	}
	return nil
}
