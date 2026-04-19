package create

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/git"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

// fakeGitClient implements gitClienter for tests.
type fakeGitClient struct {
	branch  string
	remotes []git.Remote
}

func (f *fakeGitClient) CurrentBranch(_ context.Context) (string, error) {
	return f.branch, nil
}

func (f *fakeGitClient) Remotes(_ context.Context) ([]git.Remote, error) {
	return f.remotes, nil
}

func TestCreateRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/pulls") {
			// CreatePullRequest REST call
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"number":   33,
				"title":    "Feature branch PR",
				"html_url": "https://github.com/cli/cli/pull/33",
				"draft":    false,
				"head":     map[string]interface{}{"ref": "feature"},
				"base":     map[string]interface{}{"ref": "main"},
			})
			return
		}
		// GraphQL call (GetRepository)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"id":            "R_1",
					"name":          "cli",
					"nameWithOwner": "cli/cli",
					"owner":         map[string]string{"login": "cli"},
					"description":   "",
					"isPrivate":     false,
					"isFork":        false,
					"stargazerCount":  0,
					"forkCount":     0,
					"defaultBranchRef": map[string]string{"name": "main"},
					"url":           "https://github.com/cli/cli",
				},
			},
		})
	}))
	defer srv.Close()

	originURL, _ := url.Parse("https://github.com/cli/cli.git")
	ios, _, out, _ := iostreams.Test()
	opts := &CreateOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		GitClient: &fakeGitClient{
			branch:  "feature",
			remotes: []git.Remote{{Name: "origin", FetchURL: originURL}},
		},
		Title: "Feature branch PR",
	}

	if err := createRun(opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !strings.Contains(out.String(), "#33") {
		t.Errorf("output = %q, want to contain #33", out.String())
	}
}

type rewriteTransport struct {
	base  string
	inner http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(rt.base, "http://")
	return rt.inner.RoundTrip(clone)
}
