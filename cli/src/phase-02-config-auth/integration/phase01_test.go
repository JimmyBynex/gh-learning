package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// binaryPath returns the path at which to build/find the gh binary.
func binaryPath(t *testing.T) string {
	t.Helper()
	// Locate the module root (one directory up from this file's package).
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../src/phase-02-config-auth/integration/phase01_test.go
	// module root  = ..  relative to thisFile's dir (integration/)
	moduleRoot := filepath.Join(filepath.Dir(thisFile), "..")

	binName := "gh"
	if runtime.GOOS == "windows" {
		binName = "gh.exe"
	}
	return filepath.Join(moduleRoot, "bin", binName)
}

// buildBinary compiles the gh binary once per test run.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := binaryPath(t)

	// Ensure output directory exists.
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	_, thisFile, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Join(filepath.Dir(thisFile), "..")

	cmd := exec.Command("go", "build", "-o", bin, "./cmd/gh/")
	cmd.Dir = moduleRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestIntegration_Version(t *testing.T) {
	bin := buildBinary(t)

	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "gh version") {
		t.Errorf("--version output = %q, want to contain 'gh version'", got)
	}
}

func TestIntegration_VersionSubcmd(t *testing.T) {
	bin := buildBinary(t)

	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		t.Fatalf("version subcommand failed: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "gh version") {
		t.Errorf("version subcommand output = %q, want to contain 'gh version'", got)
	}
}

func TestIntegration_Help(t *testing.T) {
	bin := buildBinary(t)

	out, err := exec.Command(bin, "--help").Output()
	if err != nil {
		t.Fatalf("--help failed: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "GitHub CLI") {
		t.Errorf("--help output = %q, want to contain 'GitHub CLI'", got)
	}
}

func TestIntegration_NoArgs(t *testing.T) {
	bin := buildBinary(t)

	// Running with no args should show help (exits 0 since RunE calls Help()).
	out, err := exec.Command(bin).Output()
	if err != nil {
		// exit 1 is also acceptable if the command exits without showing help
		t.Logf("no-args exit error: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "GitHub CLI") && !strings.Contains(got, "gh") {
		t.Errorf("no-args output = %q, want to mention 'GitHub CLI' or 'gh'", got)
	}
}

func TestIntegration_UnknownCommand(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "totally-unknown-command-xyz")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit for unknown command")
	}
	got := string(out)
	if !strings.Contains(got, "unknown command") {
		t.Errorf("unknown command output = %q, want to contain 'unknown command'", got)
	}
}
