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

// ViewOptions holds all inputs for the pr view command.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Repo       string
	PRArg      string
}

// NewCmdPRView creates the `gh pr view` command.
func NewCmdPRView(f *cmdutil.Factory) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}
	cmd := &cobra.Command{
		Use:   "view <number>",
		Short: "View details of a pull request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.PRArg = args[0]
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
	number, err := strconv.Atoi(opts.PRArg)
	if err != nil {
		return fmt.Errorf("invalid pull request number: %q", opts.PRArg)
	}
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)
	pr, err := api.GetPullRequest(client, repo, number)
	if err != nil {
		return err
	}
	printPR(opts.IO, pr)
	return nil
}

func printPR(io *iostreams.IOStreams, pr *api.PullRequest) {
	w := io.Out
	draft := ""
	if pr.IsDraft {
		draft = " [DRAFT]"
	}
	fmt.Fprintf(w, "#%d %s%s\n", pr.Number, pr.Title, draft)
	fmt.Fprintf(w, "state:\t%s\n", strings.ToLower(pr.State))
	fmt.Fprintf(w, "author:\t%s\n", pr.Author.Login)
	fmt.Fprintf(w, "branch:\t%s -> %s\n", pr.HeadRefName, pr.BaseRefName)
	if pr.Body != "" {
		fmt.Fprintf(w, "\n%s\n", pr.Body)
	}
	fmt.Fprintf(w, "\nurl:\t%s\n", pr.URL)
}
