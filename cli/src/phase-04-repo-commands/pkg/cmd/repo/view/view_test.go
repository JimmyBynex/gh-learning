package view

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

func TestViewRun_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"id":              "R_1",
					"name":            "cli",
					"nameWithOwner":   "cli/cli",
					"owner":           map[string]string{"login": "cli"},
					"description":     "GitHub CLI",
					"isPrivate":       false,
					"isFork":          false,
					"stargazerCount":  35000,
					"forkCount":       2000,
					"defaultBranchRef": map[string]string{"name": "trunk"},
					"url":             "https://github.com/cli/cli",
				},
			},
		})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()

	opts := &ViewOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{
				Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport},
			}, nil
		},
		RepoArg: "cli/cli",
	}

	if err := viewRun(opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "cli/cli") {
		t.Errorf("output missing repo name: %q", output)
	}
	if !strings.Contains(output, "35000") {
		t.Errorf("output missing star count: %q", output)
	}
	if !strings.Contains(output, "GitHub CLI") {
		t.Errorf("output missing description: %q", output)
	}
}

func TestViewRun_MissingArg(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &ViewOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return http.DefaultClient, nil },
		RepoArg:    "",
	}
	if err := viewRun(opts); err == nil {
		t.Fatal("expected error for missing repo arg")
	}
}

// rewriteTransport redirects all requests to a base URL for testing.
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
