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
					"issues": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"number": 3, "title": "Test issue", "state": "OPEN",
								"body": "", "author": map[string]string{"login": "user"},
								"labels":    map[string]interface{}{"nodes": []interface{}{}},
								"assignees": map[string]interface{}{"nodes": []interface{}{}},
								"createdAt": "2024-01-01T00:00:00Z",
								"updatedAt": "2024-01-01T00:00:00Z",
								"url":       "https://github.com/cli/cli/issues/3",
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
	if !strings.Contains(out.String(), "Test issue") {
		t.Errorf("output = %q, want to contain 'Test issue'", out.String())
	}
}

func TestListRun_InvalidState(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &ListOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return http.DefaultClient, nil },
		Repo:       "cli/cli",
		State:      "invalid",
		Limit:      10,
	}
	if err := listRun(opts); err == nil {
		t.Fatal("expected error for invalid state")
	}
}

func TestListRun_NoRepo(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &ListOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return http.DefaultClient, nil },
		Repo:       "",
		State:      "open",
		Limit:      10,
	}
	if err := listRun(opts); err == nil {
		t.Fatal("expected error for missing repo")
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
