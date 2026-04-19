package login

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// memConfig is an in-memory cmdutil.Config for tests.
type memConfig struct {
	hosts map[string]struct{ token, user string }
}

func newMemConfig() *memConfig {
	return &memConfig{hosts: make(map[string]struct{ token, user string })}
}

func (c *memConfig) Get(hostname, key string) (string, error) {
	h := c.hosts[hostname]
	switch key {
	case "oauth_token":
		return h.token, nil
	case "user":
		return h.user, nil
	}
	return "", nil
}

func (c *memConfig) Set(hostname, key, value string) error {
	h := c.hosts[hostname]
	switch key {
	case "oauth_token":
		h.token = value
	case "user":
		h.user = value
	}
	c.hosts[hostname] = h
	return nil
}

func (c *memConfig) Write() error { return nil }

func (c *memConfig) Hosts() []string {
	out := make([]string, 0, len(c.hosts))
	for h := range c.hosts {
		out = append(out, h)
	}
	return out
}

func (c *memConfig) AuthToken(hostname string) (string, error) {
	return c.hosts[hostname].token, nil
}

func (c *memConfig) Login(hostname, username, token string) error {
	c.hosts[hostname] = struct{ token, user string }{token: token, user: username}
	return nil
}

func (c *memConfig) Logout(hostname string) error {
	delete(c.hosts, hostname)
	return nil
}

// Verify memConfig implements cmdutil.Config.
var _ cmdutil.Config = (*memConfig)(nil)

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

// makeUserServer returns an httptest.Server responding to /user.
func makeUserServer(t *testing.T, login string, code int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if code == http.StatusOK {
			_ = json.NewEncoder(w).Encode(struct {
				Login string `json:"login"`
			}{Login: login})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLoginRun_WithToken_Success(t *testing.T) {
	srv := makeUserServer(t, "alice", http.StatusOK)
	cfg := newMemConfig()
	ios, _, out, _ := iostreams.Test()

	opts := &LoginOptions{
		IO:        ios,
		Config:    func() (cmdutil.Config, error) { return cfg, nil },
		HttpClient: func() (*http.Client, error) {
			return &http.Client{
				Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport},
			}, nil
		},
		Hostname:  "github.com",
		Token:     "ghp_faketoken",
		WithToken: true,
	}

	if err := loginRun(opts); err != nil {
		t.Fatalf("loginRun: %v", err)
	}

	if !strings.Contains(out.String(), "alice") {
		t.Errorf("output = %q, want to contain username 'alice'", out.String())
	}
	if cfg.hosts["github.com"].token != "ghp_faketoken" {
		t.Errorf("stored token = %q", cfg.hosts["github.com"].token)
	}
}

func TestLoginRun_WithToken_BadToken(t *testing.T) {
	srv := makeUserServer(t, "", http.StatusUnauthorized)
	cfg := newMemConfig()
	ios, _, _, _ := iostreams.Test()

	opts := &LoginOptions{
		IO:        ios,
		Config:    func() (cmdutil.Config, error) { return cfg, nil },
		HttpClient: func() (*http.Client, error) {
			return &http.Client{
				Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport},
			}, nil
		},
		Hostname:  "github.com",
		Token:     "ghp_bad",
		WithToken: true,
	}

	if err := loginRun(opts); err == nil {
		t.Fatal("expected error for bad token")
	}
}

func TestLoginRun_DeviceFlow_Success(t *testing.T) {
	// Build a mock server that handles all three Device Flow endpoints.
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			DeviceCode      string `json:"device_code"`
			UserCode        string `json:"user_code"`
			VerificationURI string `json:"verification_uri"`
			ExpiresIn       int    `json:"expires_in"`
			Interval        int    `json:"interval"`
		}{"dev_code", "TEST-0000", "https://github.com/login/device", 900, 1})
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
		}{"ghp_device_tok", "bearer"})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Login string `json:"login"`
		}{"deviceuser"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := newMemConfig()
	ios, inBuf, _, _ := iostreams.Test()
	inBuf.WriteString("\n") // simulate Enter keypress

	opts := &LoginOptions{
		IO:       ios,
		Config:   func() (cmdutil.Config, error) { return cfg, nil },
		HttpClient: func() (*http.Client, error) {
			// Redirect all requests (github.com + api.github.com) to the test server.
			return &http.Client{
				Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport},
			}, nil
		},
		Hostname:  "github.com",
		WithToken: false,
	}

	// rewriteTransport redirects all HTTP traffic to srv.URL, so both the
	// github.com OAuth endpoints and the api.github.com /user endpoint are served by srv.
	if err := loginRun(opts); err != nil {
		t.Fatalf("loginRun device flow: %v", err)
	}
	if cfg.hosts["github.com"].token != "ghp_device_tok" {
		t.Errorf("stored token = %q, want ghp_device_tok", cfg.hosts["github.com"].token)
	}
	if cfg.hosts["github.com"].user != "deviceuser" {
		t.Errorf("stored user = %q, want deviceuser", cfg.hosts["github.com"].user)
	}
}

func TestAPIBaseURL(t *testing.T) {
	tests := []struct {
		hostname string
		want     string
	}{
		{"github.com", "https://api.github.com"},
		{"ghes.example.com", "https://ghes.example.com/api/v3"},
		{"mygithub.corp", "https://mygithub.corp/api/v3"},
	}
	for _, tc := range tests {
		got := apiBaseURL(tc.hostname)
		if got != tc.want {
			t.Errorf("apiBaseURL(%q) = %q, want %q", tc.hostname, got, tc.want)
		}
	}
}

func TestNewCmdLogin_WithToken_ReadsStdin(t *testing.T) {
	srv := makeUserServer(t, "alice", http.StatusOK)
	cfg := newMemConfig()
	ios, inBuf, out, _ := iostreams.Test()
	// Provide token on stdin followed by newline (as the user would pipe: echo token | gh auth login --with-token).
	inBuf.WriteString("ghp_faketoken\n")

	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:   func() (cmdutil.Config, error) { return cfg, nil },
		HttpClient: func() (*http.Client, error) {
			return &http.Client{
				Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport},
			}, nil
		},
	}

	// Execute under a parent command so cobra doesn't conflict on the -h shorthand
	// that login uses for --hostname (in production login runs under `gh auth`).
	parent := &cobra.Command{Use: "auth"}
	parent.AddCommand(NewCmdLogin(f))
	parent.SetArgs([]string{"login", "--with-token"})
	parent.SetIn(ios.In)
	parent.SetOut(ios.Out)
	parent.SetErr(ios.ErrOut)
	if err := parent.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "alice") {
		t.Errorf("output = %q, want to contain 'alice'", out.String())
	}
	if cfg.hosts["github.com"].token != "ghp_faketoken" {
		t.Errorf("stored token = %q, want ghp_faketoken", cfg.hosts["github.com"].token)
	}
}
