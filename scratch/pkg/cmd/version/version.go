package version

import (
	"fmt"
	"io"
	"scratch/pkg/cmdutil"
	"strings"

	"github.com/spf13/cobra"
)

// VersionOptions holds the dependencies for the version command.
type VersionOptions struct {
	Out        io.Writer
	VersionStr string
}

// NewCmdVersion returns a hidden cobra command that prints the version string.
func NewCmdVersion(f *cmdutil.Factory, ver string) *cobra.Command {
	opts := &VersionOptions{
		Out:        f.IOStreams.Out,
		VersionStr: Format(ver),
	}
	return &cobra.Command{
		Use:    "version",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return versionRun(opts)
		},
	}
}

func versionRun(opts *VersionOptions) error {
	fmt.Fprint(opts.Out, opts.VersionStr)
	return nil
}

// Format formats a version string into the standard gh version output.
func Format(ver string) string {
	ver = strings.TrimPrefix(ver, "v")
	return fmt.Sprintf("gh version %s\nhttps://github.com/cli/cli/releases/tag/v%s\n", ver, ver)
}
