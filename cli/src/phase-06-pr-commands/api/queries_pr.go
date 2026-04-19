package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/learngh/gh-impl/internal/ghrepo"
)

// PullRequest represents a GitHub pull request.
type PullRequest struct {
	Number      int
	Title       string
	State       string
	Body        string
	HeadRefName string
	BaseRefName string
	Author      struct{ Login string }
	URL         string
	CreatedAt   time.Time
	IsDraft     bool
}

const prFields = `
			number
			title
			state
			body
			headRefName
			baseRefName
			author { login }
			url
			createdAt
			isDraft`

// ListPullRequests fetches pull requests for a repository.
func ListPullRequests(client *Client, repo ghrepo.Interface, state string, limit int) ([]PullRequest, error) {
	if state == "" {
		state = "OPEN"
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var result struct {
		Repository struct {
			PullRequests struct {
				Nodes []PullRequest
			}
		}
	}
	query := `
query ListPullRequests($owner: String!, $name: String!, $states: [PullRequestState!], $first: Int!) {
	repository(owner: $owner, name: $name) {
		pullRequests(states: $states, first: $first, orderBy: {field: CREATED_AT, direction: DESC}) {
			nodes {` + prFields + `
			}
		}
	}
}`
	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"name":   repo.RepoName(),
		"states": []string{state},
		"first":  limit,
	}
	if err := client.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}
	return result.Repository.PullRequests.Nodes, nil
}

// GetPullRequest fetches a single pull request by number.
func GetPullRequest(client *Client, repo ghrepo.Interface, number int) (*PullRequest, error) {
	var result struct {
		Repository struct {
			PullRequest PullRequest
		}
	}
	query := `
query GetPullRequest($owner: String!, $name: String!, $number: Int!) {
	repository(owner: $owner, name: $name) {
		pullRequest(number: $number) {` + prFields + `
		}
	}
}`
	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"name":   repo.RepoName(),
		"number": number,
	}
	if err := client.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}
	return &result.Repository.PullRequest, nil
}

// CreatePullRequest creates a new pull request via REST POST.
// params should contain: title, body, head (branch), base (branch).
func CreatePullRequest(client *Client, repo *Repository, params map[string]interface{}) (*PullRequest, error) {
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("repos/%s/%s/pulls", repo.RepoOwner(), repo.RepoName())
	var raw struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		IsDraft bool   `json:"draft"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err := client.REST(repo.RepoHost(), "POST", path, bytes.NewReader(b), &raw); err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}
	return &PullRequest{
		Number:      raw.Number,
		Title:       raw.Title,
		URL:         raw.HTMLURL,
		IsDraft:     raw.IsDraft,
		HeadRefName: raw.Head.Ref,
		BaseRefName: raw.Base.Ref,
	}, nil
}
