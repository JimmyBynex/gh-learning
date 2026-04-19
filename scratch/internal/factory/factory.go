package factory

import (
	"errors"
	"net/http"
	"scratch/api"
	"scratch/internal/config"
	"scratch/pkg/cmdutil"
	"scratch/pkg/iostreams"
)

type stubConfig struct{}

func (s *stubConfig) Get(hostname, key string) (string, error) {
	return "", errors.New("Config not implemented")
}

func (s *stubConfig) Set(hostname, key, value string) error {
	return errors.New("Config not implemented")
}

func (s *stubConfig) Write() error {
	return errors.New("Config not implemented")
}

func (s *stubConfig) Hosts() []string {
	return nil
}

func (s *stubConfig) AuthToken(hostname string) (string, error) {
	return "", errors.New("Config not implemented")
}

func (s *stubConfig) Login(hostname, username, token string) error {
	return errors.New("Config not implemented")
}

func (s *stubConfig) Logout(hostname string) error {
	return errors.New("Config not implemented")
}

// New constructs a Factory with sensible defaults for the given version.
// Auth commands override the Config getter with the real config.NewConfig().
func New(appVersion string) *cmdutil.Factory {
	ios := iostreams.System()
	f := &cmdutil.Factory{
		IOStreams:      ios,
		AppVersion:     appVersion,
		ExecutableName: "gh",
		Config: func() (cmdutil.Config, error) {
			return config.NewConfig()
		},
	}
	f.HttpClient = func() (*http.Client, error) {
		cfg, err := f.Config()
		if err != nil {
			return api.NewHTTPClient("", f.AppVersion), nil
		}
		token, _ := cfg.AuthToken("github.com")
		return api.NewHTTPClient(token, f.AppVersion), nil
	}
	return f
}
