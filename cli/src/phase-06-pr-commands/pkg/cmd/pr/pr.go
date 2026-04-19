package pr

import (
	"github.com/learngh/gh-impl/pkg/cmd/pr/create"
	"github.com/learngh/gh-impl/pkg/cmd/pr/list"
	"github.com/learngh/gh-impl/pkg/cmd/pr/view"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdPR creates the `gh pr` command group.
func NewCmdPR(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <subcommand>",
		Short: "Manage pull requests",
		Long:  "Work with GitHub pull requests.",
	}
	cmd.AddCommand(list.NewCmdPRList(f))
	cmd.AddCommand(view.NewCmdPRView(f))
	cmd.AddCommand(create.NewCmdPRCreate(f))
	return cmd
}
