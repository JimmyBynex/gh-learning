package issue

import (
	"github.com/learngh/gh-impl/pkg/cmd/issue/create"
	"github.com/learngh/gh-impl/pkg/cmd/issue/list"
	"github.com/learngh/gh-impl/pkg/cmd/issue/view"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdIssue creates the `gh issue` command group.
func NewCmdIssue(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <subcommand>",
		Short: "Manage issues",
		Long:  "Work with GitHub issues.",
	}
	cmd.AddCommand(list.NewCmdIssueList(f))
	cmd.AddCommand(view.NewCmdIssueView(f))
	cmd.AddCommand(create.NewCmdIssueCreate(f))
	return cmd
}
