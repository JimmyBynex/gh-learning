package create

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/git"
	"github.com/learngh/gh-impl/internal/ghrepo"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// gitClienter abstracts git operations needed by pr create.
type gitClienter interface {
	CurrentBranch(ctx context.Context) (string, error)
	Remotes(ctx context.Context) ([]git.Remote, error)
}

// CreateOptions holds all inputs for the pr create command.
type CreateOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	GitClient  gitClienter
	Repo       string
	Title      string
	Body       string
	Base       string
	Draft      bool
}

// NewCmdPRCreate creates the `gh pr create` command.
func NewCmdPRCreate(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient, // *git.Client satisfies gitClienter
	}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		RunE: func(cmd *cobra.Command, args []string) error {
			return createRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Title (required)")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Body")
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "The branch into which you want your code merged")
	cmd.Flags().BoolVarP(&opts.Draft, "draft", "d", false, "Mark pull request as a draft")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func createRun(opts *CreateOptions) error {
	ctx := context.Background()

	if opts.GitClient == nil {
		return fmt.Errorf("not in a git repository")
	}

	headBranch, err := opts.GitClient.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("could not determine head branch: %w", err)
	}

	var repo ghrepo.Interface
	if opts.Repo != "" {
		repo, err = ghrepo.FromFullName(opts.Repo)
		if err != nil {
			return err
		}
	} else {
		remotes, err := opts.GitClient.Remotes(ctx)
		if err != nil || len(remotes) == 0 {
			return fmt.Errorf("no git remotes found; use --repo")
		}
		var fetchURL *url.URL
		for _, r := range remotes {
			if r.Name == "origin" {
				fetchURL = r.FetchURL
				break
			}
		}
		if fetchURL == nil {
			fetchURL = remotes[0].FetchURL
		}
		if fetchURL == nil {
			return fmt.Errorf("remote has no fetch URL; use --repo")
		}
		repo, err = ghrepo.FromURL(fetchURL)
		if err != nil {
			return fmt.Errorf("could not parse remote URL: %w", err)
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	apiRepo, err := api.GetRepository(client, repo)
	if err != nil {
		return err
	}

	base := opts.Base
	if base == "" {
		base = apiRepo.DefaultBranchRef.Name
		if base == "" {
			base = "main"
		}
	}

	params := map[string]interface{}{
		"title": opts.Title,
		"body":  opts.Body,
		"head":  headBranch,
		"base":  base,
		"draft": opts.Draft,
	}

	pr, err := api.CreatePullRequest(client, apiRepo, params)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.Out, "Created pull request #%d: %s\n%s\n", pr.Number, pr.Title, pr.URL)
	return nil
}
