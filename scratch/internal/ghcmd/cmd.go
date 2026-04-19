package ghcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"scratch/internal/build"
	"scratch/internal/factory"
	"scratch/pkg/cmd/root"
	"scratch/pkg/cmdutil"
	"strings"

	"github.com/spf13/cobra"
)

type exitCode int

const (
	exitOK     exitCode = 0
	exitError  exitCode = 1
	exitCancel exitCode = 2
	exitAuth   exitCode = 4
)

// Main is the primary entry point for the gh CLI. It returns an exit code
func Main() exitCode {
	//stubVersion and stubFactory
	buildVersion := build.Version
	cmdFactory := factory.New(buildVersion)
	stderr := cmdFactory.IOStreams.ErrOut

	rootCmd, err := root.NewCmdRoot(cmdFactory, buildVersion)
	if err != nil {
		fmt.Fprintf(stderr, "failed to create root command: %s\n", err)
		return exitError
	}

	rootCmd.SetArgs(os.Args[1:])

	if cmd, err := rootCmd.ExecuteC(); err != nil {
		var authError *root.AuthError
		//先检查是不是静默退出，如果是，返回1
		//接着检查是不是用户自己取消了，如果是，返回2
		//接着检查是不是登录错误
		//最后才是检查是外部错误还是flag错误
		if errors.Is(err, cmdutil.SilentError) {
			return exitError
		} else if cmdutil.IsUserCancellation(err) {
			return exitCancel
		} else if errors.As(err, &authError) {
			return exitAuth
		}
		printError(stderr, err, cmd)
		return exitError
	}
	return exitOK
}

// printError writes err to out.When the error looks like an unknown command or a flag error
func printError(stderr io.Writer, err error, cmd *cobra.Command) {
	//先直接输出错误
	fmt.Fprintln(stderr, err)

	var flagError *cmdutil.FlagError
	//接着检查是否为flag错误，或者是未知指令错误
	if errors.As(err, &flagError) || strings.HasPrefix(err.Error(), "unknown command ") {
		if !strings.HasSuffix(err.Error(), "\n") {
			//这里是api返回错误或者是网络失败，不打印
			fmt.Fprintln(stderr)
		}
		// 打印usage
		fmt.Fprintln(stderr, cmd.Usage())
	}

}
