package list

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

func TestListRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"pullRequests": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"number": 5, "title": "Cool PR", "state": "OPEN",
								"body": "", "headRefName": "feature", "baseRefName": "main",
								"author":    map[string]string{"login": "user"},
								"url":       "https://github.com/cli/cli/pull/5",
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

	ios, _, out, _ := iostreams.Test()
	opts := &ListOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Repo:  "cli/cli",
		State: "open",
		Limit: 10,
	}
	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "Cool PR") {
		t.Errorf("output = %q", out.String())
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
