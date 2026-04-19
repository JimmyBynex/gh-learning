package list

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

type memConfig struct {
	token string
}

func (c *memConfig) Get(hostname, key string) (string, error)    { return "", nil }
func (c *memConfig) Set(hostname, key, value string) error       { return nil }
func (c *memConfig) Write() error                                 { return nil }
func (c *memConfig) Hosts() []string                              { return nil }
func (c *memConfig) AuthToken(hostname string) (string, error)    { return c.token, nil }
func (c *memConfig) Login(hostname, username, token string) error { return nil }
func (c *memConfig) Logout(hostname string) error                 { return nil }

var _ cmdutil.Config = (*memConfig)(nil)

func TestListRun_WithLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repositoryOwner": map[string]interface{}{
					"repositories": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"id": "R_1", "name": "myrepo", "nameWithOwner": "alice/myrepo",
								"owner": map[string]string{"login": "alice"},
								"description": "", "isPrivate": false, "isFork": false,
								"stargazerCount": 0, "forkCount": 0,
								"defaultBranchRef": map[string]string{"name": "main"},
								"url": "https://github.com/alice/myrepo",
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()
	opts := &ListOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{
				Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport},
			}, nil
		},
		Config: func() (cmdutil.Config, error) { return &memConfig{token: "tok"}, nil },
		Login:  "alice",
		Limit:  10,
	}

	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "alice/myrepo") {
		t.Errorf("output missing repo: %q", out.String())
	}
}

// rewriteTransport redirects all requests to a base URL for testing.
type rewriteTransport struct {
	base  string
	inner http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(rt.base, "http://")
	return rt.inner.RoundTrip(clone)
}
