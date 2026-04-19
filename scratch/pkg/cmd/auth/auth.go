package auth

import (
	"scratch/internal/config"
	"scratch/pkg/cmd/auth/login"
	"scratch/pkg/cmd/auth/status"
	"scratch/pkg/cmdutil"

	"github.com/spf13/cobra"
)

// NewCmdAuth builds the `gh auth` command and registers its subcommands.
// It wraps the factory's Config getter with the real file-backed config so
// that auth commands can read and write ~/.config/gh/hosts.yml.
func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	// Build a copy of the factory with the real config getter.
	// The stub in factory.New is replaced here so auth commands use the disk.
	//值复制而非指针复制，方便修改config而不影响其他函数
	authFactory := *f
	authFactory.Config = func() (cmdutil.Config, error) {
		return config.NewConfig()
	}

	cmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Authenticate gh and git with GitHub",
		Long:  "Authenticate gh and git with GitHub.",
	}

	cmd.AddCommand(login.NewCmdLogin(&authFactory))
	cmd.AddCommand(status.NewCmdStatus(&authFactory))

	return cmd
}
