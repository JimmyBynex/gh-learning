package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/learngh/gh-impl/internal/ghrepo"
)

func TestListIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"issues": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"number": 1, "title": "Bug report", "state": "OPEN",
								"body": "Something broke", "author": map[string]string{"login": "alice"},
								"labels":    map[string]interface{}{"nodes": []interface{}{}},
								"assignees": map[string]interface{}{"nodes": []interface{}{}},
								"createdAt": "2024-01-01T00:00:00Z",
								"updatedAt": "2024-01-02T00:00:00Z",
								"url":       "https://github.com/cli/cli/issues/1",
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

	issues, err := ListIssues(client, repo, "OPEN", 10)
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("len = %d, want 1", len(issues))
	}
	if issues[0].Title != "Bug report" {
		t.Errorf("Title = %q, want 'Bug report'", issues[0].Title)
	}
}

func TestGetIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"issue": map[string]interface{}{
						"number": 42, "title": "Feature request", "state": "OPEN",
						"body": "Please add X", "author": map[string]string{"login": "bob"},
						"labels":    map[string]interface{}{"nodes": []interface{}{}},
						"assignees": map[string]interface{}{"nodes": []interface{}{}},
						"createdAt": "2024-01-01T00:00:00Z",
						"updatedAt": "2024-01-01T00:00:00Z",
						"url":       "https://github.com/cli/cli/issues/42",
					},
				},
			},
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	client := NewClientFromHTTP(&http.Client{Transport: transport})
	repo := ghrepo.New("cli", "cli")

	issue, err := GetIssue(client, repo, 42)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.Number != 42 {
		t.Errorf("Number = %d, want 42", issue.Number)
	}
	if issue.Title != "Feature request" {
		t.Errorf("Title = %q", issue.Title)
	}
}

func TestCreateIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"number":   99,
			"title":    "New issue",
			"html_url": "https://github.com/cli/cli/issues/99",
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	client := NewClientFromHTTP(&http.Client{Transport: transport})
	repo := ghrepo.New("cli", "cli")

	issue, err := CreateIssue(client, repo, map[string]interface{}{
		"title": "New issue",
		"body":  "Details",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if issue.Number != 99 {
		t.Errorf("Number = %d, want 99", issue.Number)
	}
}
