// Package api provides the `gh api` command.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	apiPkg "github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// APIOptions holds dependencies and inputs for `gh api`.
type APIOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (cmdutil.Config, error)
	HttpClient  func() (*http.Client, error)
	Hostname    string
	Method      string
	RequestPath string
	IsGraphQL   bool
	Fields      map[string]string // -f key=value pairs
}

// NewCmdAPI returns the `gh api` cobra command.
func NewCmdAPI(f *cmdutil.Factory) *cobra.Command {
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
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RequestPath = args[0]
			opts.IsGraphQL = strings.ToLower(args[0]) == "graphql"
			return apiRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Method, "method", "X", "GET", "The HTTP method for the request")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "github.com", "The GitHub hostname for the request")

	// -f key=value flags for GraphQL variables / request fields.
	cmd.Flags().StringToStringVarP(&opts.Fields, "field", "f", map[string]string{}, "Add a key=value field to the request")

	return cmd
}

// apiRun executes the API request.
func apiRun(opts *APIOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := apiPkg.NewClientFromHTTP(httpClient)

	if opts.IsGraphQL {
		return runGraphQL(opts, client)
	}
	return runREST(opts, client)
}

func runREST(opts *APIOptions, client *apiPkg.Client) error {
	var data interface{}
	if err := client.REST(opts.Hostname, opts.Method, opts.RequestPath, nil, &data); err != nil {
		return fmt.Errorf("api call failed: %w", err)
	}
	return printJSON(opts.IO.Out, data)
}

func runGraphQL(opts *APIOptions, client *apiPkg.Client) error {
	query, ok := opts.Fields["query"]
	if !ok || query == "" {
		return fmt.Errorf("graphql requires -f query=<gql query>")
	}

	// Remaining fields become variables.
	variables := map[string]interface{}{}
	for k, v := range opts.Fields {
		if k != "query" {
			variables[k] = v
		}
	}
	if len(variables) == 0 {
		variables = nil
	}

	var data interface{}
	if err := client.GraphQL(opts.Hostname, query, variables, &data); err != nil {
		return fmt.Errorf("graphql call failed: %w", err)
	}
	return printJSON(opts.IO.Out, data)
}

func printJSON(w io.Writer, data interface{}) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(b))
	return nil
}
