package cmdutil

import (
	"net/http"

	"github.com/learngh/gh-impl/git"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

// Factory holds the dependencies that commands need to execute.
// All fields are set by factory.New and are safe to read concurrently.
type Factory struct {
	// AppVersion is the CLI version string (e.g. "2.40.0").
	AppVersion string
	// ExecutableName is the name used to invoke the CLI (e.g. "gh").
	ExecutableName string
	// IOStreams provides access to stdin/stdout/stderr.
	IOStreams *iostreams.IOStreams
	// Config is a lazy getter for application configuration.
	Config func() (Config, error)
	// HttpClient is a lazy getter for an authenticated HTTP client.
	HttpClient func() (*http.Client, error)
	// GitClient provides git operations.
	GitClient *git.Client
}
