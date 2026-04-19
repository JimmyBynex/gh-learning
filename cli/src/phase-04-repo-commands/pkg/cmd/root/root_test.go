package root_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/cmd/root"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

func newTestFactory(t *testing.T) *cmdutil.Factory {
	t.Helper()
	ios, _, _, _ := iostreams.Test()
	return &cmdutil.Factory{
		AppVersion:     "test",
		ExecutableName: "gh",
		IOStreams:       ios,
		Config:         func() (cmdutil.Config, error) { return nil, nil },
		HttpClient:     nil,
	}
}

func TestNewCmdRoot_returnsCommand(t *testing.T) {
	f := newTestFactory(t)
	cmd, err := root.NewCmdRoot(f, "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
}

func TestNewCmdRoot_useString(t *testing.T) {
	f := newTestFactory(t)
	cmd, _ := root.NewCmdRoot(f, "1.0.0")
	if !strings.HasPrefix(cmd.Use, "gh") {
		t.Errorf("Use = %q, want to start with 'gh'", cmd.Use)
	}
}

func TestNewCmdRoot_shortDescription(t *testing.T) {
	f := newTestFactory(t)
	cmd, _ := root.NewCmdRoot(f, "1.0.0")
	if cmd.Short != "GitHub CLI" {
		t.Errorf("Short = %q, want 'GitHub CLI'", cmd.Short)
	}
}

func TestNewCmdRoot_versionAnnotation(t *testing.T) {
	f := newTestFactory(t)
	cmd, _ := root.NewCmdRoot(f, "2.5.0")
	info, ok := cmd.Annotations["versionInfo"]
	if !ok {
		t.Fatal("missing versionInfo annotation")
	}
	if !strings.Contains(info, "gh version 2.5.0") {
		t.Errorf("versionInfo = %q, want to contain 'gh version 2.5.0'", info)
	}
}

func TestNewCmdRoot_silenceErrors(t *testing.T) {
	f := newTestFactory(t)
	cmd, _ := root.NewCmdRoot(f, "1.0.0")
	if !cmd.SilenceErrors {
		t.Error("expected SilenceErrors = true")
	}
	if !cmd.SilenceUsage {
		t.Error("expected SilenceUsage = true")
	}
}

func TestNewCmdRoot_hasVersionSubcommand(t *testing.T) {
	f := newTestFactory(t)
	cmd, _ := root.NewCmdRoot(f, "1.0.0")
	var found bool
	for _, sub := range cmd.Commands() {
		if sub.Use == "version" {
			found = true
			if !sub.Hidden {
				t.Error("version subcommand should be hidden")
			}
		}
	}
	if !found {
		t.Error("expected a 'version' subcommand")
	}
}

func TestNewCmdRoot_helpFlag(t *testing.T) {
	f := newTestFactory(t)
	cmd, _ := root.NewCmdRoot(f, "1.0.0")
	flag := cmd.PersistentFlags().Lookup("help")
	if flag == nil {
		t.Error("expected --help flag on root command")
	}
}

func TestNewCmdRoot_versionFlag(t *testing.T) {
	f := newTestFactory(t)
	cmd, _ := root.NewCmdRoot(f, "1.0.0")
	flag := cmd.Flags().Lookup("version")
	if flag == nil {
		t.Error("expected --version flag on root command")
	}
}

func TestNewCmdRoot_versionOutput(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	f := &cmdutil.Factory{
		AppVersion:     "3.1.0",
		ExecutableName: "gh",
		IOStreams:       ios,
		Config:         func() (cmdutil.Config, error) { return nil, nil },
	}

	cmd, err := root.NewCmdRoot(f, "3.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "gh version 3.1.0") {
		t.Errorf("--version output = %q, want 'gh version 3.1.0'", got)
	}
}

func TestNewCmdRoot_helpOutput(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	f := &cmdutil.Factory{
		AppVersion:     "1.0.0",
		ExecutableName: "gh",
		IOStreams:       ios,
		Config:         func() (cmdutil.Config, error) { return nil, nil },
	}

	cmd, err := root.NewCmdRoot(f, "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Redirect cobra's own output to our buffer so help text is captured.
	cmd.SetOut(out)
	cmd.SetArgs([]string{"--help"})
	// --help causes cobra to print help and return nil
	_ = cmd.Execute()

	got := out.String()
	if !strings.Contains(got, "GitHub CLI") {
		t.Errorf("--help output = %q, want to contain 'GitHub CLI'", got)
	}
}

func TestNewCmdRoot_unknownCommand(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		AppVersion:     "1.0.0",
		ExecutableName: "gh",
		IOStreams:       ios,
		Config:         func() (cmdutil.Config, error) { return nil, nil },
	}

	cmd, _ := root.NewCmdRoot(f, "1.0.0")
	cmd.SetArgs([]string{"nonexistent-command"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("error = %q, want 'unknown command'", err.Error())
	}
}

func TestAuthError_message(t *testing.T) {
	err := root.NewAuthError(errors.New("not authenticated"))
	if err.Error() != "not authenticated" {
		t.Errorf("Error() = %q, want 'not authenticated'", err.Error())
	}
}
