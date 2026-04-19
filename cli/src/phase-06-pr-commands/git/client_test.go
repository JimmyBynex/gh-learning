package git

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

// initTestRepo creates a temporary git repo with a commit and an "origin" remote,
// returning a Client pointed at it.
func initTestRepo(t *testing.T) *Client {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// Set minimal git config so commits work in CI.
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("commit", "--allow-empty", "-m", "init")
	run("remote", "add", "origin", "https://github.com/owner/repo.git")

	return &Client{RepoDir: dir}
}

func TestCurrentBranch(t *testing.T) {
	c := initTestRepo(t)
	branch, err := c.CurrentBranch(context.Background())
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "main" {
		t.Errorf("branch = %q, want main", branch)
	}
}

func TestRemotes(t *testing.T) {
	c := initTestRepo(t)
	remotes, err := c.Remotes(context.Background())
	if err != nil {
		t.Fatalf("Remotes: %v", err)
	}
	if len(remotes) != 1 {
		t.Fatalf("len(remotes) = %d, want 1", len(remotes))
	}
	if remotes[0].Name != "origin" {
		t.Errorf("remote name = %q, want origin", remotes[0].Name)
	}
	if remotes[0].FetchURL == nil {
		t.Error("FetchURL is nil")
	}
}

func TestParseRemotes(t *testing.T) {
	input := "origin\thttps://github.com/owner/repo.git (fetch)\norigin\thttps://github.com/owner/repo.git (push)\n"
	remotes := parseRemotes(input)
	if len(remotes) != 1 {
		t.Fatalf("len = %d, want 1", len(remotes))
	}
	if remotes[0].Name != "origin" {
		t.Errorf("name = %q", remotes[0].Name)
	}
	if remotes[0].FetchURL.Host != "github.com" {
		t.Errorf("host = %q", remotes[0].FetchURL.Host)
	}
}
