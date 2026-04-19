package create

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

func TestCreateRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"number":   55,
			"title":    "My issue",
			"html_url": "https://github.com/cli/cli/issues/55",
		})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()
	opts := &CreateOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Repo:  "cli/cli",
		Title: "My issue",
		Body:  "body text",
	}

	if err := createRun(opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !strings.Contains(out.String(), "#55") {
		t.Errorf("output = %q, want to contain #55", out.String())
	}
}

func TestCreateRun_NoRepo(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &CreateOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return http.DefaultClient, nil },
		Repo:       "",
		Title:      "My issue",
	}
	if err := createRun(opts); err == nil {
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
