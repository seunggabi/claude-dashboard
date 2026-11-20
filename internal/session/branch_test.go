package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitBranch(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	repo := filepath.Join(root, "repo")
	write(filepath.Join(repo, ".git", "HEAD"), "ref: refs/heads/feat/x\n")
	sub := filepath.Join(repo, "src", "pkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	detached := filepath.Join(root, "detached")
	write(filepath.Join(detached, ".git", "HEAD"), "0123456789abcdef0123456789abcdef01234567\n")

	worktree := filepath.Join(root, "wt")
	write(filepath.Join(worktree, ".git"), "gitdir: ../repo/.git/worktrees/wt\n")
	write(filepath.Join(repo, ".git", "worktrees", "wt", "HEAD"), "ref: refs/heads/wt-branch\n")

	cases := map[string]string{
		repo:                         "feat/x",
		sub:                          "feat/x",
		detached:                     "0123456",
		worktree:                     "wt-branch",
		filepath.Join(root, "plain"): "",
		"":                           "",
	}
	for dir, want := range cases {
		if got := gitBranch(dir); got != want {
			t.Errorf("gitBranch(%q) = %q, want %q", dir, got, want)
		}
	}
}
