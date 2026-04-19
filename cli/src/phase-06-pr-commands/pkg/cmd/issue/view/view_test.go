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
					"issue": map[string]interface{}{
						"number": 7, "title": "Example issue", "state": "OPEN",
						"body": "Issue body text", "author": map[string]string{"login": "dave"},
						"labels":    map[string]interface{}{"nodes": []interface{}{}},
						"assignees": map[string]interface{}{"nodes": []interface{}{}},
						"createdAt": "2024-01-01T00:00:00Z",
						"updatedAt": "2024-01-01T00:00:00Z",
						"url":       "https://github.com/cli/cli/issues/7",
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
		Repo:     "cli/cli",
		IssueArg: "7",
	}

	if err := viewRun(opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Example issue") {
		t.Errorf("output missing title: %q", output)
	}
	if !strings.Contains(output, "dave") {
		t.Errorf("output missing author: %q", output)
	}
}

func TestViewRun_InvalidNumber(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &ViewOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return http.DefaultClient, nil },
		Repo:       "cli/cli",
		IssueArg:   "notanumber",
	}
	if err := viewRun(opts); err == nil {
		t.Fatal("expected error for invalid number")
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
