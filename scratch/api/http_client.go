package api

import "net/http"

const apiVersionValue = "2022-11-28"

// authTransport implements adding authorization
type authTransport struct {
	token string
	inner http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token != "" && req.Header.Get("Authorization") == "" {
		//复制一份完全相同的请求，这是因为go的http请求过程中不允许修改
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	//这是所谓的装饰器模式，拦截原本请求加上装饰后再调用
	return t.inner.RoundTrip(req)
}

// userAgentTransport implements adding user-agent
type userAgentTransport struct {
	agent string
	inner http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.agent != "" && req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.agent)
	}
	return t.inner.RoundTrip(req)
}

// apiVersionTransport implements adding api version
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
	tr = &userAgentTransport{agent: appVersion, inner: tr}
	if token != "" {
		tr = &authTransport{token: token, inner: tr}
	}
	return &http.Client{Transport: tr}
}
