package api

import (
	"fmt"
	"net/http"
)

const apiVersionValue = "2022-11-28"

type authTransport struct {
	token string
	inner http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token != "" && req.Header.Get("Authorization") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.inner.RoundTrip(req)
}

type userAgentTransport struct {
	agent string
	inner http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.agent)
	}
	return t.inner.RoundTrip(req)
}

type apiVersionTransport struct {
	inner http.RoundTripper
}

func (t *apiVersionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("X-GitHub-Api-Version") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("X-GitHub-Api-Version", apiVersionValue)
	}
	return t.inner.RoundTrip(req)
}

// NewHTTPClient returns an *http.Client pre-configured with:
//   - Authorization: Bearer <token>  (if token non-empty)
//   - User-Agent: GitHub CLI <appVersion>
//   - X-GitHub-Api-Version: 2022-11-28
func NewHTTPClient(token, appVersion string) *http.Client {
	tr := http.RoundTripper(http.DefaultTransport)
	tr = &apiVersionTransport{inner: tr}
	tr = &userAgentTransport{agent: fmt.Sprintf("GitHub CLI %s", appVersion), inner: tr}
	if token != "" {
		tr = &authTransport{token: token, inner: tr}
	}
	return &http.Client{Transport: tr}
}
