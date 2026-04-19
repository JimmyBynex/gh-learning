package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"gopkg.in/yaml.v3"
)

// hostEntry holds per-hostname authentication data.
type hostEntry struct {
	OAuthToken string `yaml:"oauth_token,omitempty"`
	User       string `yaml:"user,omitempty"`
}

// fileConfig is the file-backed implementation of cmdutil.Config.
// hosts maps hostname -> hostEntry.
type fileConfig struct {
	mu    sync.Mutex
	hosts map[string]*hostEntry
	path  string // absolute path to hosts.yml
}

// ConfigDir returns the directory that holds gh configuration files.
func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "gh")
}

// hostsFilePath returns the path to the hosts.yml config file.
func hostsFilePath() string {
	return filepath.Join(ConfigDir(), "hosts.yml")
}

// NewConfig reads ~/.config/gh/hosts.yml and returns a Config implementation.
// If the file does not exist, an empty config is returned without error.
func NewConfig() (cmdutil.Config, error) {
	return newConfigFromPath(hostsFilePath())
}

// newConfigFromPath reads the given path and returns a Config. Used by tests
// to inject a temporary file path instead of the real config location.
func newConfigFromPath(path string) (cmdutil.Config, error) {
	hosts := make(map[string]*hostEntry)

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &hosts); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
	}

	// yaml.Unmarshal may leave nil entries for keys with no sub-keys.
	for k, v := range hosts {
		if v == nil {
			hosts[k] = &hostEntry{}
		}
	}

	return &fileConfig{hosts: hosts, path: path}, nil
}

// Get returns the value for key under hostname.
// Supported keys: "oauth_token", "user".
func (c *fileConfig) Get(hostname, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.hosts[hostname]
	if !ok {
		return "", fmt.Errorf("hostname %q not found in config", hostname)
	}
	switch key {
	case "oauth_token":
		return entry.OAuthToken, nil
	case "user":
		return entry.User, nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

// Set stores value for key under hostname.
func (c *fileConfig) Set(hostname, key, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.hosts[hostname]
	if !ok {
		entry = &hostEntry{}
		c.hosts[hostname] = entry
	}
	switch key {
	case "oauth_token":
		entry.OAuthToken = value
	case "user":
		entry.User = value
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

// Write persists the current in-memory config to disk.
// The caller must NOT hold mu when calling Write.
func (c *fileConfig) Write() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeUnlocked()
}

// writeUnlocked persists the config without acquiring the mutex.
// The caller must hold mu.
func (c *fileConfig) writeUnlocked() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(c.hosts)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.WriteFile(c.path, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// Hosts returns all configured hostnames.
func (c *fileConfig) Hosts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	names := make([]string, 0, len(c.hosts))
	for h := range c.hosts {
		names = append(names, h)
	}
	return names
}

// AuthToken returns the OAuth token stored for hostname.
func (c *fileConfig) AuthToken(hostname string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.hosts[hostname]
	if !ok || entry.OAuthToken == "" {
		return "", fmt.Errorf("not logged in to %s", hostname)
	}
	return entry.OAuthToken, nil
}

// Login stores username and token for hostname and writes to disk atomically
// under the same lock acquisition.
func (c *fileConfig) Login(hostname, username, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hosts[hostname] = &hostEntry{
		OAuthToken: token,
		User:       username,
	}
	return c.writeUnlocked()
}

// Logout removes credentials for hostname and writes to disk atomically
// under the same lock acquisition.
func (c *fileConfig) Logout(hostname string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.hosts, hostname)
	return c.writeUnlocked()
}
