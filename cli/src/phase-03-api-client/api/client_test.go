package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestREST_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/user" {
			t.Errorf("path = %q, want /user", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"login": "alice"})
	}))
	defer srv.Close()

	client := NewClientFromHTTP(srv.Client())
	// Override URL: use a custom transport that rewrites the host.
	var data struct{ Login string }
	// Direct test using the actual REST helper via a rewrite transport.
	httpCl := &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}
	c := NewClientFromHTTP(httpCl)
	if err := c.REST("github.com", http.MethodGet, "/user", nil, &data); err != nil {
		t.Fatalf("REST: %v", err)
	}
	if data.Login != "alice" {
		t.Errorf("Login = %q, want alice", data.Login)
	}
	_ = client
}

func TestREST_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"message": "requires authentication"})
	}))
	defer srv.Close()

	httpCl := &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}
	c := NewClientFromHTTP(httpCl)
	err := c.REST("github.com", http.MethodGet, "/user", nil, nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	var httpErr HTTPError
	if e, ok := err.(HTTPError); ok {
		httpErr = e
	} else {
		t.Fatalf("expected HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", httpErr.StatusCode)
	}
	if httpErr.Message != "requires authentication" {
		t.Errorf("Message = %q", httpErr.Message)
	}
}

func TestHTTPError_Error(t *testing.T) {
	e := HTTPError{StatusCode: 404, Message: "Not Found"}
	if e.Error() != "HTTP 404: Not Found" {
		t.Errorf("Error() = %q", e.Error())
	}
	e2 := HTTPError{StatusCode: 500}
	if e2.Error() != "HTTP 500" {
		t.Errorf("Error() = %q", e2.Error())
	}
}

func TestGraphQL_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			t.Errorf("path = %q, want /graphql", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]string{"login": "alice"},
			},
		})
	}))
	defer srv.Close()

	httpCl := &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}
	c := NewClientFromHTTP(httpCl)

	var data struct {
		Viewer struct{ Login string }
	}
	err := c.GraphQL("github.com", `query { viewer { login } }`, nil, &data)
	if err != nil {
		t.Fatalf("GraphQL: %v", err)
	}
	if data.Viewer.Login != "alice" {
		t.Errorf("Login = %q, want alice", data.Viewer.Login)
	}
}

func TestGraphQL_errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{
				{"message": "Field 'foo' doesn't exist"},
			},
		})
	}))
	defer srv.Close()

	httpCl := &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}
	c := NewClientFromHTTP(httpCl)

	err := c.GraphQL("github.com", `query { foo }`, nil, nil)
	if err == nil {
		t.Fatal("expected GraphQL error")
	}
	gqlErr, ok := err.(GraphQLError)
	if !ok {
		t.Fatalf("expected GraphQLError, got %T", err)
	}
	if gqlErr.Message != "Field 'foo' doesn't exist" {
		t.Errorf("Message = %q", gqlErr.Message)
	}
}

func TestGraphQLError_Error(t *testing.T) {
	e := GraphQLError{Message: "some graphql error"}
	if e.Error() != "some graphql error" {
		t.Errorf("Error() = %q", e.Error())
	}
}

// rewriteTransport redirects all requests to a test server base URL.
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
