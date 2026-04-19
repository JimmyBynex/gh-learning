package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strings"
)

// Remote represents a git remote.
type Remote struct {
	Name     string
	FetchURL *url.URL
	PushURL  *url.URL
}

// Client runs git commands in a repository directory.
type Client struct {
	RepoDir string
	GitPath string // path to git binary; if empty, uses "git" from PATH
	Stderr  io.Writer
}

// gitPath returns the git binary path, defaulting to "git".
func (c *Client) gitPath() string {
	if c.GitPath != "" {
		return c.GitPath
	}
	return "git"
}

// run executes a git command and returns its stdout.
func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.gitPath(), args...)
	if c.RepoDir != "" {
		cmd.Dir = c.RepoDir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	if c.Stderr != nil {
		cmd.Stderr = c.Stderr
	} else {
		cmd.Stderr = &stderr
	}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// CurrentBranch returns the name of the current git branch.
func (c *Client) CurrentBranch(ctx context.Context) (string, error) {
	branch, err := c.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("could not determine current branch: %w", err)
	}
	if branch == "HEAD" {
		return "", fmt.Errorf("not on a branch (detached HEAD state)")
	}
	return branch, nil
}

// Remotes returns the configured git remotes.
func (c *Client) Remotes(ctx context.Context) ([]Remote, error) {
	out, err := c.run(ctx, "remote", "-v")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return parseRemotes(out), nil
}

// parseRemotes parses the output of `git remote -v`.
// Each remote appears twice (fetch and push); we deduplicate by name.
func parseRemotes(output string) []Remote {
	seen := map[string]*Remote{}
	var order []string

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "origin\thttps://github.com/owner/repo.git (fetch)"
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		name := parts[0]
		rawURL := parts[1]
		kind := strings.Trim(parts[2], "()")

		u, err := url.Parse(rawURL)
		if err != nil {
			continue
		}

		r, ok := seen[name]
		if !ok {
			r = &Remote{Name: name}
			seen[name] = r
			order = append(order, name)
		}
		switch kind {
		case "fetch":
			r.FetchURL = u
		case "push":
			r.PushURL = u
		}
	}

	remotes := make([]Remote, 0, len(order))
	for _, name := range order {
		remotes = append(remotes, *seen[name])
	}
	return remotes
}
