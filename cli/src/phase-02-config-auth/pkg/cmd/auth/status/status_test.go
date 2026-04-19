package status

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

// memConfig is an in-memory Config for tests.
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
	h, ok := c.hosts[hostname]
	if !ok || h.token == "" {
		return "", fmt.Errorf("not logged in to %s", hostname)
	}
	return h.token, nil
}

func (c *memConfig) Login(hostname, username, token string) error {
	c.hosts[hostname] = struct{ token, user string }{token: token, user: username}
	return nil
}

func (c *memConfig) Logout(hostname string) error {
	delete(c.hosts, hostname)
	return nil
}

var _ cmdutil.Config = (*memConfig)(nil)

func TestStatusRun_NoHosts(t *testing.T) {
	cfg := newMemConfig()
	ios, _, _, errOut := iostreams.Test()

	opts := &StatusOptions{
		IO:     ios,
		Config: func() (cmdutil.Config, error) { return cfg, nil },
		HttpClient: func() (*http.Client, error) {
			return &http.Client{}, nil
		},
	}

	err := statusRun(opts)
	if err == nil {
		t.Fatal("expected error when no hosts are configured")
	}
	if !strings.Contains(errOut.String(), "not logged in") {
		t.Errorf("errOut = %q, want to contain 'not logged in'", errOut.String())
	}
}

func TestStatusRun_WithHost(t *testing.T) {
	cfg := newMemConfig()
	_ = cfg.Login("github.com", "alice", "ghp_token123")

	ios, _, out, _ := iostreams.Test()

	opts := &StatusOptions{
		IO:     ios,
		Config: func() (cmdutil.Config, error) { return cfg, nil },
		HttpClient: func() (*http.Client, error) {
			return &http.Client{}, nil
		},
	}

	if err := statusRun(opts); err != nil {
		t.Fatalf("statusRun: %v", err)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "github.com") {
		t.Errorf("output = %q, want to contain 'github.com'", outStr)
	}
	if !strings.Contains(outStr, "alice") {
		t.Errorf("output = %q, want to contain 'alice'", outStr)
	}
}

func TestStatusRun_MultipleHosts(t *testing.T) {
	cfg := newMemConfig()
	_ = cfg.Login("github.com", "alice", "ghp_tok1")
	_ = cfg.Login("ghe.example.com", "bob", "ghs_tok2")

	ios, _, out, _ := iostreams.Test()

	opts := &StatusOptions{
		IO:     ios,
		Config: func() (cmdutil.Config, error) { return cfg, nil },
		HttpClient: func() (*http.Client, error) {
			return &http.Client{}, nil
		},
	}

	if err := statusRun(opts); err != nil {
		t.Fatalf("statusRun: %v", err)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "alice") || !strings.Contains(outStr, "bob") {
		t.Errorf("output = %q, want to contain both usernames", outStr)
	}
}
