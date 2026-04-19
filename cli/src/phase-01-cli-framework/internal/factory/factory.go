package factory

import (
	"errors"
	"net/http"

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

// New constructs a Factory with sensible defaults for the given version.
// Auth commands override the Config getter with the real config.NewConfig().
func New(appVersion string) *cmdutil.Factory {
	ios := iostreams.System()

	f := &cmdutil.Factory{
		AppVersion:     appVersion,
		ExecutableName: "gh",
		IOStreams:       ios,
		Config: func() (cmdutil.Config, error) {
			return &stubConfig{}, nil
		},
		HttpClient: func() (*http.Client, error) {
			return &http.Client{}, nil
		},
		GitClient: nil,
	}

	return f
}
