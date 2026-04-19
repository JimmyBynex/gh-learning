package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/learngh/gh-impl/internal/ghrepo"
)

func TestGetRepository(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"id":            "R_123",
					"name":          "cli",
					"nameWithOwner": "cli/cli",
					"owner":         map[string]string{"login": "cli"},
					"description":   "GitHub CLI",
					"isPrivate":     false,
					"isFork":        false,
					"stargazerCount": 1000,
					"forkCount":     200,
					"defaultBranchRef": map[string]string{"name": "main"},
					"url":           "https://github.com/cli/cli",
				},
			},
		})
	}))
	defer srv.Close()

	// rewriteTransport is defined in client_test.go (same package).
	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	httpClient := &http.Client{Transport: transport}
	client := NewClientFromHTTP(httpClient)

	repo := ghrepo.New("cli", "cli")
	result, err := GetRepository(client, repo)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if result.Name != "cli" {
		t.Errorf("Name = %q, want cli", result.Name)
	}
	if result.StargazerCount != 1000 {
		t.Errorf("StargazerCount = %d, want 1000", result.StargazerCount)
	}
}

func TestListRepositories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repositoryOwner": map[string]interface{}{
					"repositories": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"id": "R_1", "name": "repo1", "nameWithOwner": "alice/repo1",
								"owner": map[string]string{"login": "alice"},
								"description": "Repo 1", "isPrivate": false, "isFork": false,
								"stargazerCount": 5, "forkCount": 0,
								"defaultBranchRef": map[string]string{"name": "main"},
								"url": "https://github.com/alice/repo1",
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	httpClient := &http.Client{Transport: transport}
	client := NewClientFromHTTP(httpClient)

	repos, err := ListRepositories(client, "alice", 10)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
	if repos[0].Name != "repo1" {
		t.Errorf("repos[0].Name = %q, want repo1", repos[0].Name)
	}
}
