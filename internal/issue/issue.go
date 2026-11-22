// Package issue fetches GitHub issues via the gh CLI and builds typed
// instructions for Claude Code sessions.
package issue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// ownerNameRe validates a single GitHub owner or repository name component.
var ownerNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// repoRe matches "owner/name" as a whole.
var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// ParseRepoFromRemote extracts "owner/name" from a git remote URL:
// git@github.com:o/n.git, https://github.com/o/n(.git)(/),
// ssh://git@github.com/o/n.git, with or without a trailing slash/.git.
// ok is false for non-GitHub hosts or malformed URLs.
// Owner/name must match ^[A-Za-z0-9_.-]+$.
func ParseRepoFromRemote(rawURL string) (repo string, ok bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", false
	}

	// SCP-style: git@github.com:owner/name.git
	if strings.HasPrefix(rawURL, "git@github.com:") {
		path := strings.TrimPrefix(rawURL, "git@github.com:")
		owner, name, valid := parseOwnerName(path)
		if !valid {
			return "", false
		}
		return owner + "/" + name, true
	}

	// URL-style: https://github.com/... or ssh://git@github.com/...
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	if u.Hostname() != "github.com" {
		return "", false
	}
	path := strings.TrimPrefix(u.Path, "/")
	owner, name, valid := parseOwnerName(path)
	if !valid {
		return "", false
	}
	return owner + "/" + name, true
}

// parseOwnerName splits "owner/name[.git][/]" into owner and name, validating each.
func parseOwnerName(path string) (owner, name string, ok bool) {
	// Strip optional trailing slash then optional .git then optional trailing slash again.
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimSuffix(path, "/")

	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	owner, name = parts[0], parts[1]
	// Name itself must not contain further slashes (SplitN(2) already guarantees that).
	if !ownerNameRe.MatchString(owner) || !ownerNameRe.MatchString(name) {
		return "", "", false
	}
	return owner, name, true
}

// Runner executes a command and returns stdout; injectable for tests.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// DefaultRunner runs the real command with exec.CommandContext.
func DefaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Issue represents a GitHub issue.
type Issue struct {
	Repo   string
	Number int
	Title  string
	URL    string
}

// ghIssueJSON is the JSON shape returned by gh issue view --json.
type ghIssueJSON struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

// Fetch loads an issue with `gh issue view <n> -R <repo> --json number,title,url`.
// Repo (owner/name) and number (>0) are validated BEFORE running anything.
// Title is returned sanitized (control chars removed, whitespace collapsed,
// truncated to 80 runes) because it is only a human label.
func Fetch(ctx context.Context, run Runner, repo string, number int) (Issue, error) {
	if !repoRe.MatchString(repo) {
		return Issue{}, fmt.Errorf("invalid repo %q: expected owner/name", repo)
	}
	if number <= 0 {
		return Issue{}, fmt.Errorf("issue number must be positive, got %d", number)
	}

	out, err := run(ctx, "gh", "issue", "view", strconv.Itoa(number), "-R", repo, "--json", "number,title,url")
	if err != nil {
		return Issue{}, fmt.Errorf("gh issue view failed: %w", err)
	}

	var j ghIssueJSON
	if err := json.Unmarshal(out, &j); err != nil {
		return Issue{}, fmt.Errorf("failed to parse gh output: %w", err)
	}

	return Issue{
		Repo:   repo,
		Number: j.Number,
		Title:  sanitizeTitle(j.Title),
		URL:    j.URL,
	}, nil
}

// sanitizeTitle removes control characters, collapses whitespace, and
// truncates to 80 runes.
func sanitizeTitle(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			sb.WriteRune(' ')
		} else {
			sb.WriteRune(r)
		}
	}
	result := strings.Join(strings.Fields(sb.String()), " ")
	runes := []rune(result)
	if len(runes) > 80 {
		result = string(runes[:80])
	}
	return result
}

// ParseRef parses "123", "#123", "owner/name#123" and a full
// https://github.com/o/n/issues/123 URL.
// repo is "" when the ref does not name one (caller supplies it).
func ParseRef(ref string) (repo string, number int, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", 0, fmt.Errorf("empty issue ref")
	}

	// Full GitHub issue URL: https://github.com/owner/name/issues/123
	if strings.HasPrefix(ref, "https://github.com/") {
		u, parseErr := url.Parse(ref)
		if parseErr != nil {
			return "", 0, fmt.Errorf("invalid URL: %w", parseErr)
		}
		// Path: /owner/name/issues/123
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 4 || parts[2] != "issues" {
			return "", 0, fmt.Errorf("invalid GitHub issue URL: %s", ref)
		}
		owner, repoName := parts[0], parts[1]
		if !ownerNameRe.MatchString(owner) || !ownerNameRe.MatchString(repoName) {
			return "", 0, fmt.Errorf("invalid owner or repo name in URL: %s", ref)
		}
		n, parseErr := parsePositiveInt(parts[3])
		if parseErr != nil {
			return "", 0, fmt.Errorf("invalid issue number in URL: %s", ref)
		}
		return owner + "/" + repoName, n, nil
	}

	// "owner/name#123" or "#123"
	if idx := strings.IndexByte(ref, '#'); idx >= 0 {
		repoStr := ref[:idx]
		numStr := ref[idx+1:]
		if repoStr != "" {
			if !repoRe.MatchString(repoStr) {
				return "", 0, fmt.Errorf("invalid repo %q in ref", repoStr)
			}
			n, parseErr := parsePositiveInt(numStr)
			if parseErr != nil {
				return "", 0, parseErr
			}
			return repoStr, n, nil
		}
		// "#123"
		n, parseErr := parsePositiveInt(numStr)
		if parseErr != nil {
			return "", 0, parseErr
		}
		return "", n, nil
	}

	// Plain number "123"
	n, parseErr := parsePositiveInt(ref)
	if parseErr != nil {
		return "", 0, parseErr
	}
	return "", n, nil
}

// parsePositiveInt parses a positive integer from a string containing only
// ASCII digits (no sign, no spaces, no shell metacharacters).
func parsePositiveInt(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty issue number")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid issue number %q: must be a positive integer", s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid issue number %q: %w", s, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("issue number must be positive, got %d", n)
	}
	return n, nil
}

// maxPromptBytes is the maximum allowed size of the prompt, mirroring
// tmux.MaxSendLength.
const maxPromptBytes = 4000

// Prompt builds the ONE-LINE instruction typed into a session.
//
// SECURITY: the result contains only the repo and the issue NUMBER and fixed
// wording — never the title or any other issue text, because those are
// untrusted and may carry prompt-injection payloads.
//
// If guidelines (extra text) is non-empty it is appended after a space.
// The result must be a single line without control characters and at most
// 4000 bytes; Prompt returns an error otherwise.
//
// Example output:
//
//	Work on GitHub issue #123 in acme/app: read it with `gh issue view 123`, create a new branch, implement it with tests, and open a pull request that closes it.
func (i Issue) Prompt(guidelines string) (string, error) {
	base := fmt.Sprintf(
		"Work on GitHub issue #%d in %s: read it with `gh issue view %d`, create a new branch, implement it with tests, and open a pull request that closes it.",
		i.Number, i.Repo, i.Number,
	)
	result := base
	if guidelines != "" {
		result = base + " " + guidelines
	}
	if err := validatePrompt(result); err != nil {
		return "", err
	}
	return result, nil
}

// validatePrompt mirrors tmux.ValidateSendText without importing internal/tmux.
// A valid prompt is non-empty, at most maxPromptBytes bytes, and contains no
// control characters (which includes newlines and tabs).
func validatePrompt(text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("prompt is empty")
	}
	if len(text) > maxPromptBytes {
		return fmt.Errorf("prompt is %d bytes; the limit is %d", len(text), maxPromptBytes)
	}
	for _, r := range text {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("prompt must be a single line without control characters")
		}
	}
	return nil
}

// OriginRepo returns the "owner/name" of the git remote "origin" of dir,
// or "" on any failure (not a repo, no origin, non-GitHub).
// Matches the dispatch.RepoResolver signature func(dir string) string.
func OriginRepo(dir string) string {
	out, err := exec.CommandContext(context.Background(), "git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	repo, ok := ParseRepoFromRemote(strings.TrimSpace(string(out)))
	if !ok {
		return ""
	}
	return repo
}
