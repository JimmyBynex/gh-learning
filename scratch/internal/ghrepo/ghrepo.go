package ghrepo

import (
	"fmt"
	"net/url"
	"strings"
)

// Interface represents a Github repository reference
// 设计这个接口契约，是为了服务于两种调用方式，第一个是CLI，第二个是API返回，
type Interface interface {
	RepoName() string
	RepoOwner() string
	RepoHost() string
}

type ghPepo struct {
	owner string
	name  string
	host  string
}

func (r ghPepo) RepoName() string {
	return r.name
}

func (r ghPepo) RepoHost() string {
	return r.host
}

func (r ghPepo) RepoOwner() string {
	return r.owner
}

func New(owner, name string) Interface {
	return &ghPepo{owner: owner, name: name, host: "github.com"}
}

func NewWithHost(owner, name string, host string) Interface {
	return &ghPepo{owner: owner, name: name, host: host}
}

// FullName returns "ownner/name"
func FullName(r Interface) string {
	return fmt.Sprintf("%s/%s", r.RepoOwner(), r.RepoName())
}

// FromFullName parses "owner/name" or "host/owner/name"
// Returns error if the string is not valid repo reference
func FromFullName(nwo string) (Interface, error) {
	parts := strings.Split(nwo, "/")
	switch len(parts) {
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid repository reference")
		}
		return New(parts[0], parts[1]), nil
	case 3:
		if parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("invalid repository reference")
		}
		return New(parts[0], parts[1]), nil
	default:
		return nil, fmt.Errorf("expected \"owner/name\" or \"owner/host\"")
	}
}

// FromURL parses a GitHub repository URL
// Supports https://github.com/owner/name and git@github.com:owner/name.git
func FromURL(u *url.URL) (Interface, error) {
	if u.Hostname() == "" {
		return nil, fmt.Errorf("no hostname in URL")
	}
	//先是去除前缀
	path := strings.TrimPrefix(u.Path, "/")
	//再是去除后缀
	path = strings.TrimSuffix(path, ".git")
	//接着分割
	parts := strings.SplitN(path, "/", 2)
	return NewWithHost(parts[0], parts[1], u.Host), nil

}
