// 文件：api/queries_repo.go
package api

import (
	"fmt"
	"scratch/internal/ghrepo"
)

// Repository represents a GitHub repository.
type Repository struct {
	ID            string
	Name          string
	NameWithOwner string
	//这里使用匿名结构体主要是为了方便映射
	Owner            struct{ Login string }
	Description      string
	IsPrivate        bool
	IsFork           bool
	StargazerCount   int
	ForkCount        int
	DefaultBranchRef struct{ Name string }
	URL              string
}

// RepoOwner implements ghrepo.Interface.
func (r Repository) RepoOwner() string { return r.Owner.Login }

// RepoName implements ghrepo.Interface.
func (r Repository) RepoName() string { return r.Name }

// RepoHost implements ghrepo.Interface.
func (r Repository) RepoHost() string { return "github.com" }

// GetRepository fetches a single repository by owner/name.
func GetRepository(client *Client, repo ghrepo.Interface) (*Repository, error) {
	var result struct {
		Repository Repository `json:"repository"`
	}
	query := `
query GetRepository($owner: String!, $name: String!) {
    repository(owner: $owner, name: $name) {
        id
        name
        nameWithOwner
        owner { login }
        description
        isPrivate
        isFork
        stargazerCount
        forkCount
        defaultBranchRef { name }
        url
    }
}`
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"name":  repo.RepoName(),
	}
	if err := client.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}
	return &result.Repository, nil
}

// ListRepositories lists repositories for a user.
func ListRepositories(client *Client, login string, limit int) ([]Repository, error) {
	var result struct {
		RepositoryOwner struct {
			Repositories struct {
				Nodes []Repository
			}
		}
	}
	query := `
query ListRepositories($login: String!, $first: Int!) {
    repositoryOwner(login: $login) {
        repositories(first: $first, orderBy: {field: PUSHED_AT, direction: DESC}) {
            nodes {
                id
                name
                nameWithOwner
                owner { login }
                description
                isPrivate
                isFork
                stargazerCount
                forkCount
                defaultBranchRef { name }
                url
            }
        }
    }
}`
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	variables := map[string]interface{}{
		"login": login,
		"first": limit,
	}
	if err := client.GraphQL("github.com", query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}
	return result.RepositoryOwner.Repositories.Nodes, nil
}
