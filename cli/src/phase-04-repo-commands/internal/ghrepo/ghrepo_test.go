package ghrepo

import (
	"net/url"
	"testing"
)

func TestFromFullName(t *testing.T) {
	tests := []struct {
		input   string
		owner   string
		name    string
		host    string
		wantErr bool
	}{
		{"cli/cli", "cli", "cli", "github.com", false},
		{"owner/repo", "owner", "repo", "github.com", false},
		{"github.com/owner/repo", "owner", "repo", "github.com", false},
		{"invalid", "", "", "", true},
		{"/repo", "", "", "", true},
		{"owner/", "", "", "", true},
	}
	for _, tc := range tests {
		got, err := FromFullName(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("FromFullName(%q): expected error", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("FromFullName(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if got.RepoOwner() != tc.owner || got.RepoName() != tc.name || got.RepoHost() != tc.host {
			t.Errorf("FromFullName(%q) = {%s/%s @ %s}, want {%s/%s @ %s}",
				tc.input, got.RepoOwner(), got.RepoName(), got.RepoHost(),
				tc.owner, tc.name, tc.host)
		}
	}
}

func TestFullName(t *testing.T) {
	r := New("owner", "repo")
	if got := FullName(r); got != "owner/repo" {
		t.Errorf("FullName = %q, want owner/repo", got)
	}
}

func TestFromURL(t *testing.T) {
	u, _ := url.Parse("https://github.com/cli/cli")
	r, err := FromURL(u)
	if err != nil {
		t.Fatalf("FromURL: %v", err)
	}
	if r.RepoOwner() != "cli" || r.RepoName() != "cli" {
		t.Errorf("FromURL = %s/%s, want cli/cli", r.RepoOwner(), r.RepoName())
	}
}
