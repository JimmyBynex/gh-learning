package version_test

import (
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/cmd/version"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

func TestFormat_devVersion(t *testing.T) {
	got := version.Format("DEV")
	if !strings.Contains(got, "gh version DEV") {
		t.Errorf("Format(DEV) = %q, want to contain 'gh version DEV'", got)
	}
}

func TestFormat_tagged(t *testing.T) {
	got := version.Format("v2.40.0")
	if !strings.Contains(got, "gh version 2.40.0") {
		t.Errorf("Format(v2.40.0) = %q, want to contain 'gh version 2.40.0'", got)
	}
	if !strings.Contains(got, "releases/tag/v2.40.0") {
		t.Errorf("Format(v2.40.0) = %q, want to contain release URL", got)
	}
}

func TestFormat_noVPrefix(t *testing.T) {
	got := version.Format("2.41.0")
	if !strings.Contains(got, "gh version 2.41.0") {
		t.Errorf("Format without v prefix: %q", got)
	}
}

func TestNewCmdVersion_properties(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	cmd := version.NewCmdVersion(f, "1.2.3")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if !cmd.Hidden {
		t.Error("version command should be hidden")
	}
	if cmd.Use != "version" {
		t.Errorf("Use = %q, want 'version'", cmd.Use)
	}
}

func TestNewCmdVersion_output(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	versionInfo := version.Format("1.2.3")

	// Build a minimal root command that carries the versionInfo annotation
	// so cmd.Root().Annotations["versionInfo"] works correctly.
	root := &cobra.Command{
		Use: "gh",
		Annotations: map[string]string{
			"versionInfo": versionInfo,
		},
	}
	root.SilenceErrors = true
	root.SilenceUsage = true

	vCmd := version.NewCmdVersion(f, "1.2.3")
	root.AddCommand(vCmd)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "gh version 1.2.3") {
		t.Errorf("output = %q, want to contain 'gh version 1.2.3'", got)
	}
	if !strings.Contains(got, "releases/tag/v1.2.3") {
		t.Errorf("output = %q, want to contain release URL", got)
	}
}

func TestNewCmdVersion_devBuild(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	versionInfo := version.Format("DEV")
	root := &cobra.Command{
		Use:         "gh",
		Annotations: map[string]string{"versionInfo": versionInfo},
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.AddCommand(version.NewCmdVersion(f, "DEV"))
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "gh version DEV") {
		t.Errorf("output = %q, want 'gh version DEV'", got)
	}
}
