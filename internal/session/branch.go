package session

import (
	"os"
	"path/filepath"
	"strings"
)

// gitBranch returns the branch checked out in the repository that contains
// dir, a short commit hash when HEAD is detached, or "" outside a repository.
// It reads .git/HEAD directly so a refresh does not start a git process per
// session.
func gitBranch(dir string) string {
	if dir == "" {
		return ""
	}
	for {
		gitPath := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			gitDir := gitPath
			if !info.IsDir() {
				// Worktrees and submodules: .git is a file "gitdir: <path>".
				data, err := os.ReadFile(gitPath)
				if err != nil {
					return ""
				}
				gitDir = strings.TrimSpace(strings.TrimPrefix(string(data), "gitdir:"))
				if !filepath.IsAbs(gitDir) {
					gitDir = filepath.Join(dir, gitDir)
				}
			}
			head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
			if err != nil {
				return ""
			}
			ref := strings.TrimSpace(string(head))
			if branch, ok := strings.CutPrefix(ref, "ref: refs/heads/"); ok {
				return branch
			}
			if other, ok := strings.CutPrefix(ref, "ref: "); ok {
				return other
			}
			if len(ref) > 7 {
				return ref[:7]
			}
			return ref
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
