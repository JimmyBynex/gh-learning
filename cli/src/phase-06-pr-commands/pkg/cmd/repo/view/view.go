package view

import (
	"fmt"
	"net/http"

	"github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/internal/ghrepo"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ViewOptions holds all inputs for the repo view command.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	RepoArg    string // positional: owner/name
}

// NewCmdRepoView creates the `gh repo view` command.
func NewCmdRepoView(f *cmdutil.Factory) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "view [<repository>]",
		Short: "View repository information",
		Long:  "Display details about a GitHub repository.\n\nWith no argument, shows the repository for the current directory.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			return viewRun(opts)
		},
	}
	return cmd
}

func viewRun(opts *ViewOptions) error {
	if opts.RepoArg == "" {
		return fmt.Errorf("repository argument required (e.g. gh repo view owner/name)")
	}

	repo, err := ghrepo.FromFullName(opts.RepoArg)
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	repository, err := api.GetRepository(client, repo)
	if err != nil {
		return err
	}

	printRepository(opts.IO, repository)
	return nil
}

func printRepository(io *iostreams.IOStreams, r *api.Repository) {
	w := io.Out
	fmt.Fprintf(w, "name:\t%s\n", r.NameWithOwner)
	if r.Description != "" {
		fmt.Fprintf(w, "description:\t%s\n", r.Description)
	}
	fmt.Fprintf(w, "stars:\t%d\n", r.StargazerCount)
	fmt.Fprintf(w, "forks:\t%d\n", r.ForkCount)
	visibility := "public"
	if r.IsPrivate {
		visibility = "private"
	}
	fmt.Fprintf(w, "visibility:\t%s\n", visibility)
	if r.DefaultBranchRef.Name != "" {
		fmt.Fprintf(w, "default branch:\t%s\n", r.DefaultBranchRef.Name)
	}
	fmt.Fprintf(w, "url:\t%s\n", r.URL)
}
