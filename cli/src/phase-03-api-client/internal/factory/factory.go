package factory

import (
	"errors"
	"net/http"

	apiPkg "github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

// stubConfig is a minimal Config implementation used as a placeholder until
// the real config is injected (e.g. by auth commands).
type stubConfig struct{}

func (c *stubConfig) Get(_, _ string) (string, error) {
	return "", errors.New("config not implemented")
}

func (c *stubConfig) Set(_, _, _ string) error {
	return errors.New("config not implemented")
}

func (c *stubConfig) Write() error {
	return errors.New("config not implemented")
}

func (c *stubConfig) Hosts() []string {
	return nil
}

func (c *stubConfig) AuthToken(_ string) (string, error) {
	return "", errors.New("config not implemented")
}

func (c *stubConfig) Login(_, _, _ string) error {
	return errors.New("config not implemented")
}

func (c *stubConfig) Logout(_ string) error {
	return errors.New("config not implemented")
}

// New constructs a Factory. HttpClient is lazy: reads the auth token from
// Config at call time and builds an authenticated *http.Client.
func New(appVersion string) *cmdutil.Factory {
	ios := iostreams.System()

	f := &cmdutil.Factory{
		AppVersion:     appVersion,
		ExecutableName: "gh",
		IOStreams:       ios,
		Config: func() (cmdutil.Config, error) {
			return &stubConfig{}, nil
		},
	}

	f.HttpClient = func() (*http.Client, error) {
		cfg, err := f.Config()
		if err != nil {
			return apiPkg.NewHTTPClient("", f.AppVersion), nil
		}
		// Best-effort: get token for github.com.
		// Auth commands inject their own Authorization header so they are unaffected.
		token, _ := cfg.AuthToken("github.com")
		return apiPkg.NewHTTPClient(token, f.AppVersion), nil
	}

	f.GitClient = nil
	return f
}
