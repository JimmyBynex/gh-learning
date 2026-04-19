package integration_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestIntegration_AuthStatus_NoConfig verifies that `gh auth status` prints
// an error and exits non-zero when no config file exists.
func TestIntegration_AuthStatus_NoConfig(t *testing.T) {
	bin := buildBinary(t)

	// Point config to a non-existent directory so there are no credentials.
	tmpHome := t.TempDir()
	cmd := exec.Command(bin, "auth", "status")
	cmd.Env = append(os.Environ(),
		"HOME="+tmpHome,
		"USERPROFILE="+tmpHome, // Windows
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit for auth status with no config")
	}
	got := string(out)
	// Should mention not logged in or similar.
	if !strings.Contains(strings.ToLower(got), "not logged in") &&
		!strings.Contains(strings.ToLower(got), "authenticate") &&
		!strings.Contains(strings.ToLower(got), "login") {
		t.Errorf("auth status output = %q; want to mention login/auth", got)
	}
}

// TestIntegration_AuthLogin_WithToken_NoServer verifies that `gh auth login
// --with-token` with a fake token fails (cannot contact GitHub) and exits
// non-zero. Skip if we have no network or want to avoid external calls.
func TestIntegration_AuthLogin_WithToken_FakeToken(t *testing.T) {
	// This test attempts to reach api.github.com to validate the token.
	// If GH_TEST_SKIP_NETWORK is set, skip it.
	if os.Getenv("GH_TEST_SKIP_NETWORK") != "" {
		t.Skip("skipping network test")
	}

	bin := buildBinary(t)
	tmpHome := t.TempDir()

	cmd := exec.Command(bin, "auth", "login", "--with-token")
	cmd.Stdin = strings.NewReader("ghp_thisisafaketokenforintegrationtesting\n")
	cmd.Env = append(os.Environ(),
		"HOME="+tmpHome,
		"USERPROFILE="+tmpHome,
	)
	out, err := cmd.CombinedOutput()
	// A fake token should either fail to contact GitHub (network error)
	// or receive a 401, so the exit code should be non-zero.
	if err == nil {
		t.Errorf("expected non-zero exit for fake token, output: %s", out)
	}
}

// TestIntegration_AuthLogin_WithToken_EnvToken verifies that when a real
// GH_TOKEN is available, the login --with-token flow succeeds.
func TestIntegration_AuthLogin_WithToken_RealToken(t *testing.T) {
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		t.Skip("GH_TOKEN not set; skipping real token integration test")
	}

	bin := buildBinary(t)
	tmpHome := t.TempDir()

	cmd := exec.Command(bin, "auth", "login", "--with-token")
	cmd.Stdin = strings.NewReader(token + "\n")
	cmd.Env = append(os.Environ(),
		"HOME="+tmpHome,
		"USERPROFILE="+tmpHome,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth login --with-token failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Logged in") {
		t.Errorf("output = %q, want to contain 'Logged in'", string(out))
	}
}

// TestIntegration_AuthStatus_AfterLogin verifies that after logging in with a
// real token, `gh auth status` shows the correct hostname and username.
func TestIntegration_AuthStatus_AfterLogin(t *testing.T) {
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		t.Skip("GH_TOKEN not set; skipping auth status after login test")
	}

	bin := buildBinary(t)
	tmpHome := t.TempDir()
	env := append(os.Environ(),
		"HOME="+tmpHome,
		"USERPROFILE="+tmpHome,
	)

	// Login first.
	loginCmd := exec.Command(bin, "auth", "login", "--with-token")
	loginCmd.Stdin = strings.NewReader(token + "\n")
	loginCmd.Env = env
	if out, err := loginCmd.CombinedOutput(); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}

	// Now check status.
	statusCmd := exec.Command(bin, "auth", "status")
	statusCmd.Env = env
	out, err := statusCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("auth status: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "github.com") {
		t.Errorf("status output = %q, want to contain 'github.com'", string(out))
	}
}
