package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

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

func TestAPIRun_REST_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"login": "alice"})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()
	opts := &APIOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Config:      func() (cmdutil.Config, error) { return nil, nil },
		Hostname:    "github.com",
		Method:      "GET",
		RequestPath: "/user",
		Fields:      map[string]string{},
	}

	if err := apiRun(opts); err != nil {
		t.Fatalf("apiRun: %v", err)
	}
	if !strings.Contains(out.String(), "alice") {
		t.Errorf("output = %q, want to contain 'alice'", out.String())
	}
}

func TestAPIRun_REST_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"message": "requires authentication"})
	}))
	defer srv.Close()

	ios, _, _, _ := iostreams.Test()
	opts := &APIOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Config:      func() (cmdutil.Config, error) { return nil, nil },
		Hostname:    "github.com",
		Method:      "GET",
		RequestPath: "/user",
		Fields:      map[string]string{},
	}

	err := apiRun(opts)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want to contain '401'", err.Error())
	}
}

func TestAPIRun_GraphQL_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]string{"login": "bob"},
			},
		})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()
	opts := &APIOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Config:      func() (cmdutil.Config, error) { return nil, nil },
		Hostname:    "github.com",
		RequestPath: "graphql",
		IsGraphQL:   true,
		Fields:      map[string]string{"query": "{ viewer { login } }"},
	}

	if err := apiRun(opts); err != nil {
		t.Fatalf("apiRun graphql: %v", err)
	}
	if !strings.Contains(out.String(), "bob") {
		t.Errorf("output = %q, want to contain 'bob'", out.String())
	}
}

func TestAPIRun_GraphQL_missingQuery(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &APIOptions{
		IO:          ios,
		HttpClient:  func() (*http.Client, error) { return &http.Client{}, nil },
		Config:      func() (cmdutil.Config, error) { return nil, nil },
		RequestPath: "graphql",
		IsGraphQL:   true,
		Fields:      map[string]string{},
	}
	err := apiRun(opts)
	if err == nil {
		t.Fatal("expected error for missing query")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("error = %q, want to mention 'query'", err.Error())
	}
}

func TestNewCmdAPI_hasCorrectUse(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams:   ios,
		Config:     func() (cmdutil.Config, error) { return nil, nil },
		HttpClient: func() (*http.Client, error) { return &http.Client{}, nil },
	}
	cmd := NewCmdAPI(f)
	if cmd.Use != "api <endpoint>" {
		t.Errorf("Use = %q", cmd.Use)
	}
}
