package integration_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestIntegration_API_NoToken verifies that `gh api /user` without authentication
// prints a meaningful error (not a panic) and exits non-zero.
func TestIntegration_API_NoToken(t *testing.T) {
	bin := buildBinary(t)
	tmpHome := t.TempDir()

	cmd := exec.Command(bin, "api", "/user")
	cmd.Env = append(os.Environ(),
		"HOME="+tmpHome,
		"USERPROFILE="+tmpHome, // Windows
	)
	out, err := cmd.CombinedOutput()
	// Without a token the API will return 401, so exit should be non-zero.
	if err == nil {
		t.Errorf("expected non-zero exit without auth, got: %s", out)
	}
}

// TestIntegration_API_WithToken verifies that `gh api /user` with a real GH_TOKEN succeeds.
func TestIntegration_API_WithToken(t *testing.T) {
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		t.Skip("GH_TOKEN not set; skipping API integration test")
	}

	bin := buildBinary(t)
	tmpHome := t.TempDir()

	// First login so config has the token.
	loginCmd := exec.Command(bin, "auth", "login", "--with-token")
	loginCmd.Stdin = strings.NewReader(token + "\n")
	loginCmd.Env = append(os.Environ(), "HOME="+tmpHome, "USERPROFILE="+tmpHome)
	if out, err := loginCmd.CombinedOutput(); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}

	apiCmd := exec.Command(bin, "api", "/user")
	apiCmd.Env = append(os.Environ(), "HOME="+tmpHome, "USERPROFILE="+tmpHome)
	out, err := apiCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("api /user: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "login") {
		t.Errorf("output = %q, want JSON with 'login' field", string(out))
	}
}
