package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"scratch/api"
	"scratch/pkg/cmdutil"
	"scratch/pkg/iostreams"
	"strings"

	"github.com/spf13/cobra"
)

type APIOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (cmdutil.Config, error)
	HttpClient  func() (*http.Client, error)
	Hostname    string
	Method      string
	RequestPath string
	IsGraphQL   bool
	Fields      map[string]string
}

// NewCmdApi returns the `gh api` cobra command
func NewCmdApi(f *cmdutil.Factory) *cobra.Command {
	opts := &APIOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Hostname:   "github.com",
		Method:     "GET",
		Fields:     map[string]string{},
	}
	cmd := &cobra.Command{
		Use:   "api <endpoint>",
		Short: "Make an authenticated GitHub API request",
		Long: `Make an authenticated GitHub API request.

For REST endpoints, supply a path like /user or repos/owner/repo.
For GraphQL, use the "graphql" endpoint with -f query=<gql>.`,
		//这是在检测args的长度，如果不对会打印错误
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RequestPath = args[0]
			opts.IsGraphQL = strings.ToLower(args[0]) == "graphql"
			return apiRun(opts)
		},
	}
	//记得规则 类型+var(变量)+P(短命令) 内部字段：变量地址，长，短，默认，描述
	cmd.Flags().StringVarP(&opts.Method, "method", "X", "GET", "HTTP method to use for requests")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "github.com", "GitHub Hostname")
	//非常特殊的一个flag，能够不断追加到map切片
	cmd.Flags().StringToStringVarP(&opts.Fields, "fields", "f", map[string]string{}, "Additional fields to display")

	return cmd
}

// apiRun executes the API request
func apiRun(opts *APIOptions) error {
	//先懒加载
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	//包装请求头
	client := api.NewClientFormHTTP(httpClient)
	if opts.IsGraphQL {
		return runGraphQL(opts, client)
	}
	return runREST(opts, client)
}

func runREST(opts *APIOptions, client *api.Client) error {
	var data interface{}
	if err := client.REST(opts.Hostname, opts.Method, opts.RequestPath, nil, &data); err != nil {
		return fmt.Errorf("api call failed: %w", err)
	}
	return printJSON(opts.IO.Out, data)
}

func runGraphQL(opts *APIOptions, client *api.Client) error {
	//先将query，variables切分开
	query, ok := opts.Fields["query"]
	if !ok || query == "" {
		return fmt.Errorf("graphql requires -f query=<gql query>")
	}
	variables := map[string]interface{}{}
	for k, v := range opts.Fields {
		if k != "query" {
			variables[k] = v
		}
	}
	var data interface{}
	if err := client.GraphQL(opts.Hostname, query, variables, &data); err != nil {
		return fmt.Errorf("graphql call failed: %w", err)
	}
	return printJSON(opts.IO.Out, data)
}

// printJSON prints the formatted output
func printJSON(out io.Writer, data interface{}) error {
	// json.Marshal(data)
	// 输出：{"name":"Alice","age":30}

	// json.MarshalIndent(data, "", "  ")
	// 输出：
	// {
	//   "name": "Alice",
	//   "age": 30
	// }
	b, err := json.MarshalIndent(data, "", " ")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(b))
	return nil
}
