package repo

import (
	"scratch/pkg/cmd/repo/list"
	"scratch/pkg/cmd/repo/view"
	"scratch/pkg/cmdutil"

	"github.com/spf13/cobra"
)

// NewCmdRepo creates the `gh repo` command group.
func NewCmdRepo(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo <subcommand>",
		Short: "Manage repositories",
		Long:  "Work with GitHub repositories.",
	}
	cmd.AddCommand(view.NewCmdRepoView(f))
	cmd.AddCommand(list.NewCmdRepoList(f))
	return cmd
}
