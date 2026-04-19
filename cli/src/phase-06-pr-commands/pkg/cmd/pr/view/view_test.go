package view

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

func TestViewRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"pullRequest": map[string]interface{}{
						"number": 9, "title": "Test PR", "state": "OPEN",
						"body": "PR body", "headRefName": "feat", "baseRefName": "main",
						"author":    map[string]string{"login": "eve"},
						"url":       "https://github.com/cli/cli/pull/9",
						"createdAt": "2024-01-01T00:00:00Z",
						"isDraft":   false,
					},
				},
			},
		})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()
	opts := &ViewOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Repo:  "cli/cli",
		PRArg: "9",
	}
	if err := viewRun(opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Test PR") {
		t.Errorf("output missing title: %q", output)
	}
	if !strings.Contains(output, "eve") {
		t.Errorf("output missing author: %q", output)
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
