package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/learngh/gh-impl/internal/ghrepo"
)

func TestListPullRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"pullRequests": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"number": 10, "title": "Add feature", "state": "OPEN",
								"body": "", "headRefName": "feature", "baseRefName": "main",
								"author": map[string]string{"login": "alice"},
								"url":       "https://github.com/cli/cli/pull/10",
								"createdAt": "2024-01-01T00:00:00Z",
								"isDraft":   false,
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	client := NewClientFromHTTP(&http.Client{Transport: transport})
	repo := ghrepo.New("cli", "cli")

	prs, err := ListPullRequests(client, repo, "OPEN", 10)
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("len = %d, want 1", len(prs))
	}
	if prs[0].Title != "Add feature" {
		t.Errorf("Title = %q", prs[0].Title)
	}
}

func TestGetPullRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"pullRequest": map[string]interface{}{
						"number": 42, "title": "Fix bug", "state": "MERGED",
						"body": "fixes it", "headRefName": "fix-branch", "baseRefName": "main",
						"author": map[string]string{"login": "bob"},
						"url":       "https://github.com/cli/cli/pull/42",
						"createdAt": "2024-01-01T00:00:00Z",
						"isDraft":   false,
					},
				},
			},
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	client := NewClientFromHTTP(&http.Client{Transport: transport})
	repo := ghrepo.New("cli", "cli")

	pr, err := GetPullRequest(client, repo, 42)
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	if pr.Number != 42 || pr.Title != "Fix bug" {
		t.Errorf("got %d %q", pr.Number, pr.Title)
	}
}

func TestCreatePullRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"number":   77,
			"title":    "My PR",
			"html_url": "https://github.com/cli/cli/pull/77",
			"draft":    false,
			"head":     map[string]interface{}{"ref": "feature"},
			"base":     map[string]interface{}{"ref": "main"},
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	client := NewClientFromHTTP(&http.Client{Transport: transport})
	repo := &Repository{
		Name:  "cli",
		Owner: struct{ Login string }{Login: "cli"},
	}

	pr, err := CreatePullRequest(client, repo, map[string]interface{}{
		"title": "My PR",
		"head":  "feature",
		"base":  "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if pr.Number != 77 {
		t.Errorf("Number = %d, want 77", pr.Number)
	}
}
