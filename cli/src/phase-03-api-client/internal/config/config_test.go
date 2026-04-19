package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestConfig creates a fileConfig backed by a temp dir.
func newTestConfig(t *testing.T) *fileConfig {
	t.Helper()
	dir := t.TempDir()
	return &fileConfig{
		hosts: make(map[string]*hostEntry),
		path:  filepath.Join(dir, "hosts.yml"),
	}
}

func TestConfig_EmptyByDefault(t *testing.T) {
	cfg := newTestConfig(t)
	if got := cfg.Hosts(); len(got) != 0 {
		t.Errorf("expected 0 hosts, got %v", got)
	}
}

func TestConfig_SetAndGet(t *testing.T) {
	cfg := newTestConfig(t)

	if err := cfg.Set("github.com", "oauth_token", "ghp_test"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := cfg.Set("github.com", "user", "alice"); err != nil {
		t.Fatalf("Set user: %v", err)
	}

	got, err := cfg.Get("github.com", "oauth_token")
	if err != nil || got != "ghp_test" {
		t.Errorf("Get oauth_token = %q, err=%v", got, err)
	}
	got, err = cfg.Get("github.com", "user")
	if err != nil || got != "alice" {
		t.Errorf("Get user = %q, err=%v", got, err)
	}
}

func TestConfig_Get_UnknownHost(t *testing.T) {
	cfg := newTestConfig(t)
	_, err := cfg.Get("missing.host", "user")
	if err == nil {
		t.Error("expected error for unknown hostname")
	}
}

func TestConfig_Set_UnknownKey(t *testing.T) {
	cfg := newTestConfig(t)
	err := cfg.Set("github.com", "bad_key", "value")
	if err == nil {
		t.Error("expected error for unknown key")
	}
}

func TestConfig_Get_UnknownKey(t *testing.T) {
	cfg := newTestConfig(t)
	// Create the host entry first so we reach the key-switch default branch.
	if err := cfg.Set("github.com", "oauth_token", "tok"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	_, err := cfg.Get("github.com", "bad_key")
	if err == nil {
		t.Error("expected error for unknown key in Get")
	}
}

func TestConfig_WriteAndRead(t *testing.T) {
	cfg := newTestConfig(t)
	if err := cfg.Login("github.com", "bob", "ghp_abc123"); err != nil {
		t.Fatalf("Login: %v", err)
	}

	data, err := os.ReadFile(cfg.path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "ghp_abc123") {
		t.Errorf("hosts.yml should contain token, got:\n%s", content)
	}
	if !strings.Contains(content, "bob") {
		t.Errorf("hosts.yml should contain username, got:\n%s", content)
	}
}

func TestConfig_AuthToken_Missing(t *testing.T) {
	cfg := newTestConfig(t)
	_, err := cfg.AuthToken("github.com")
	if err == nil {
		t.Error("expected error for missing token")
	}
}

func TestConfig_AuthToken_Present(t *testing.T) {
	cfg := newTestConfig(t)
	if err := cfg.Login("github.com", "user1", "tok123"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	tok, err := cfg.AuthToken("github.com")
	if err != nil {
		t.Fatalf("AuthToken: %v", err)
	}
	if tok != "tok123" {
		t.Errorf("AuthToken = %q, want tok123", tok)
	}
}

func TestConfig_Logout(t *testing.T) {
	cfg := newTestConfig(t)
	if err := cfg.Login("github.com", "user1", "tok123"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := cfg.Logout("github.com"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if hosts := cfg.Hosts(); len(hosts) != 0 {
		t.Errorf("expected 0 hosts after logout, got %v", hosts)
	}
}

func TestConfig_ParsesExistingYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.yml")
	raw := "github.com:\n    oauth_token: ghp_existing\n    user: charlie\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := newConfigFromPath(path)
	if err != nil {
		t.Fatalf("newConfigFromPath: %v", err)
	}

	tok, err := cfg.AuthToken("github.com")
	if err != nil {
		t.Fatalf("AuthToken: %v", err)
	}
	if tok != "ghp_existing" {
		t.Errorf("token = %q, want ghp_existing", tok)
	}
	user, _ := cfg.Get("github.com", "user")
	if user != "charlie" {
		t.Errorf("user = %q, want charlie", user)
	}
}
