package cmdutil

// Config defines the interface for reading and writing CLI configuration.
type Config interface {
	// Get returns the value for the given key under the given hostname.
	Get(hostname, key string) (string, error)
	// Set stores the value for the given key under the given hostname.
	Set(hostname, key, value string) error
	// Write persists configuration to disk.
	Write() error
	// Hosts returns all configured hostnames.
	Hosts() []string
	// AuthToken returns the OAuth token for the given hostname.
	AuthToken(hostname string) (string, error)
	// Login stores the authentication credentials for the given hostname.
	Login(hostname, username, token string) error
	// Logout removes the authentication credentials for the given hostname.
	Logout(hostname string) error
}
