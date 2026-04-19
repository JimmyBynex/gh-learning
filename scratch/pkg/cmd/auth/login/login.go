package login

import (
	"bufio"
	"fmt"
	"net/http"
	"scratch/internal/authflow"
	"scratch/pkg/cmdutil"
	"scratch/pkg/iostreams"
	"strings"

	"github.com/spf13/cobra"
)

type LoginOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (cmdutil.Config, error)
	HttpClient func() (*http.Client, error)
	HostName   string
	Token      string
	WithToken  bool
}

func NewCmdLogin(f *cmdutil.Factory) *cobra.Command {
	opts := &LoginOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a GitHub host",
		Long: `Authenticate with a GitHub host.

The default authentication mode is an interactive OAuth device flow.
Alternatively, pass in a token on standard input by using --with-token.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.WithToken {
				//获取token
				//缓冲流读取，直接读取一行
				reader := bufio.NewReader(opts.IO.In)
				tok, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("reading token from stdin: %w", err)
				}
				opts.Token = strings.TrimSpace(tok)
				//判断是否真正输入
				if opts.Token == "" {
					return fmt.Errorf("--with-token: no token provided on stdin")
				}
			}
			return loginRun(opts)
		},
	}
	//技巧
	cmd.Flags().Bool("help", false, "Show help for login") // 覆盖默认的 help
	cmd.Flags().Lookup("help").Hidden = true               // 藏起来，--help 还能用但不显示在帮助里
	//主要是为了空出-h
	cmd.Flags().StringVarP(&opts.HostName, "hostname", "h", "github.com", "The hostname of the GitHub instance to authenticate with")
	cmd.Flags().BoolVar(&opts.WithToken, "with-token", false, "Read token from standard input")
	return cmd
}

// loginRun preforms the actual login logic
func loginRun(opts *LoginOptions) error {
	//先是懒加载cfg，httpClient
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	//如果走--with_token，只需要获取username再保存就行了
	if opts.WithToken {
		apiBase := apiBaseURL(opts.HostName)
		username, err := authflow.FetchUserName(httpClient, apiBase, opts.Token)
		if err != nil {
			return fmt.Errorf("authenticating with token: %w", err)
		}
		if err := cfg.Login(opts.HostName, username, opts.Token); err != nil {
			return fmt.Errorf("saving credentials: %w", err)
		}
		fmt.Fprintf(opts.IO.Out, "Logged in to %s as %s\n", opts.HostName, username)
		return nil
	}

	//其他就需要走三层
	result, err := authflow.DeviceFlow(httpClient, opts.HostName, opts.IO)
	if err != nil {
		return err
	}
	if err := cfg.Login(opts.HostName, result.Username, result.Token); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}
	fmt.Fprintf(opts.IO.Out, "Logged in to %s as %s\n", opts.HostName, result.Username)
	return nil
}

// apiBaseURL returns the GitHub API base URL for the given hostname.
func apiBaseURL(hostname string) string {
	if hostname == "github.com" {
		return "https://api.github.com"
	}
	return "https://" + hostname + "/api/v3"
}
