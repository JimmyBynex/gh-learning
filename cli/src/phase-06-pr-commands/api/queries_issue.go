package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/learngh/gh-impl/internal/ghrepo"
)

// Label represents a GitHub label.
type Label struct {
	Name string
}

// User represents a GitHub user reference.
type User struct {
	Login string
}

// Issue represents a GitHub issue.
type Issue struct {
	Number    int
	Title     string
	State     string
	Body      string
	Author    struct{ Login string }
	Labels    struct{ Nodes []Label }
	Assignees struct{ Nodes []User }
	CreatedAt time.Time
	UpdatedAt time.Time
	URL       string
}

// ListIssues fetches issues for a repository.
func ListIssues(client *Client, repo ghrepo.Interface, state string, limit int) ([]Issue, error) {
	if state == "" {
		state = "OPEN"
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var result struct {
		Repository struct {
			Issues struct {
				Nodes []Issue
			}
		}
	}
	query := `
query ListIssues($owner: String!, $name: String!, $states: [IssueState!], $first: Int!) {
	repository(owner: $owner, name: $name) {
		issues(states: $states, first: $first, orderBy: {field: CREATED_AT, direction: DESC}) {
			nodes {
				number
				title
				state
				body
				author { login }
				labels(first: 10) { nodes { name } }
				assignees(first: 5) { nodes { login } }
				createdAt
				updatedAt
				url
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
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}
	return result.Repository.Issues.Nodes, nil
}

// GetIssue fetches a single issue by number.
func GetIssue(client *Client, repo ghrepo.Interface, number int) (*Issue, error) {
	var result struct {
		Repository struct {
			Issue Issue
		}
	}
	query := `
query GetIssue($owner: String!, $name: String!, $number: Int!) {
	repository(owner: $owner, name: $name) {
		issue(number: $number) {
			number
			title
			state
			body
			author { login }
			labels(first: 10) { nodes { name } }
			assignees(first: 5) { nodes { login } }
			createdAt
			updatedAt
			url
		}
	}
}`
	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"name":   repo.RepoName(),
		"number": number,
	}
	if err := client.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to get issue: %w", err)
	}
	return &result.Repository.Issue, nil
}

// issueRESTResponse captures the fields returned by the REST create-issue endpoint.
type issueRESTResponse struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"html_url"`
}

// CreateIssue creates a new issue via REST POST /repos/{owner}/{repo}/issues.
func CreateIssue(client *Client, repo ghrepo.Interface, params map[string]interface{}) (*Issue, error) {
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("repos/%s/%s/issues", repo.RepoOwner(), repo.RepoName())
	var raw issueRESTResponse
	if err := client.REST(repo.RepoHost(), "POST", path, bytes.NewReader(b), &raw); err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}
	return &Issue{Number: raw.Number, Title: raw.Title, URL: raw.URL}, nil
}
