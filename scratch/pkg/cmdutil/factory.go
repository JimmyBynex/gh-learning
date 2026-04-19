package cmdutil

import (
	"net/http"
	"scratch/pkg/iostreams"
)

// Factory holds the dependencies that command need to execute
type Factory struct {
	// AppVersion is the CLI version string(e.g."2.40.0")
	AppVersion string
	// ExecutableName is the name used to invoke the CLI
	ExecutableName string
	// Config is a lazy getter of application configuration
	// 懒函数加载是一开始启动的时候这些字段先不加载，后面按需才要，速度更快
	Config func() (Config, error)
	// IOStreams provide access to stdin/stdout/stderr
	IOStreams *iostreams.IOStreams
	// HttpClient is a lazy getter of http client
	HttpClient func() (*http.Client, error)
	// GitClient provide git operation
	GitClient any
}
