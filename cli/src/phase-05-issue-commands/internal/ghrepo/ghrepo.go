package ghrepo

import (
	"fmt"
	"net/url"
	"strings"
)

// Interface represents a GitHub repository reference.
type Interface interface {
	RepoName() string
	RepoOwner() string
	RepoHost() string
}

type ghRepo struct {
	owner string
	name  string
	host  string
}

func (r ghRepo) RepoOwner() string { return r.owner }
func (r ghRepo) RepoName() string  { return r.name }
func (r ghRepo) RepoHost() string  { return r.host }

// New creates a repo reference for github.com.
func New(owner, name string) Interface {
	return ghRepo{owner: owner, name: name, host: "github.com"}
}

// NewWithHost creates a repo reference for a specific host.
func NewWithHost(owner, name, host string) Interface {
	return ghRepo{owner: owner, name: name, host: host}
}

// FullName returns "owner/name".
func FullName(r Interface) string {
	return fmt.Sprintf("%s/%s", r.RepoOwner(), r.RepoName())
}

// FromFullName parses "owner/name" or "host/owner/name".
// Returns error if the string is not a valid repo reference.
func FromFullName(nwo string) (Interface, error) {
	parts := strings.Split(nwo, "/")
	switch len(parts) {
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid repository: %q", nwo)
		}
		return New(parts[0], parts[1]), nil
	case 3:
		if parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("invalid repository: %q", nwo)
		}
		return NewWithHost(parts[1], parts[2], parts[0]), nil
	default:
		return nil, fmt.Errorf("expected \"owner/name\" or \"host/owner/name\", got %q", nwo)
	}
}

// FromURL parses a GitHub repository URL.
// Supports https://github.com/owner/name and git@github.com:owner/name.git
func FromURL(u *url.URL) (Interface, error) {
	if u.Hostname() == "" {
		return nil, fmt.Errorf("no hostname in URL")
	}
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid GitHub URL: %s", u)
	}
	return NewWithHost(parts[0], parts[1], u.Hostname()), nil
}
