package view

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/internal/ghrepo"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ViewOptions holds all inputs for the issue view command.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Repo       string
	IssueArg   string // number as string from positional arg
}

// NewCmdIssueView creates the `gh issue view` command.
func NewCmdIssueView(f *cmdutil.Factory) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "view <number>",
		Short: "View details of an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IssueArg = args[0]
			return viewRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Select another repository using the [HOST/]OWNER/REPO format")
	return cmd
}

func viewRun(opts *ViewOptions) error {
	if opts.Repo == "" {
		return fmt.Errorf("repository required: use --repo owner/name")
	}
	repo, err := ghrepo.FromFullName(opts.Repo)
	if err != nil {
		return err
	}

	number, err := strconv.Atoi(opts.IssueArg)
	if err != nil {
		return fmt.Errorf("invalid issue number: %q", opts.IssueArg)
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	issue, err := api.GetIssue(client, repo, number)
	if err != nil {
		return err
	}

	printIssue(opts.IO, issue)
	return nil
}

func printIssue(io *iostreams.IOStreams, issue *api.Issue) {
	w := io.Out
	fmt.Fprintf(w, "#%d %s\n", issue.Number, issue.Title)
	fmt.Fprintf(w, "state:\t%s\n", strings.ToLower(issue.State))
	fmt.Fprintf(w, "author:\t%s\n", issue.Author.Login)
	if len(issue.Labels.Nodes) > 0 {
		labels := make([]string, len(issue.Labels.Nodes))
		for i, l := range issue.Labels.Nodes {
			labels[i] = l.Name
		}
		fmt.Fprintf(w, "labels:\t%s\n", strings.Join(labels, ", "))
	}
	if issue.Body != "" {
		fmt.Fprintf(w, "\n%s\n", issue.Body)
	}
	fmt.Fprintf(w, "\nurl:\t%s\n", issue.URL)
}
