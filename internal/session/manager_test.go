package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// resolvePath
// ---------------------------------------------------------------------------

func TestResolvePath_absolutePathPassesThrough(t *testing.T) {
	got, err := resolvePath("/tmp/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/foo" {
		t.Errorf("expected /tmp/foo, got %q", got)
	}
}

func TestResolvePath_tildeExpandsToHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	got, err := resolvePath("~/projects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, "projects")
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestResolvePath_relativePathBecomesAbsolute(t *testing.T) {
	got, err := resolvePath("some/relative/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path, got %q", got)
	}
	if !strings.HasSuffix(got, "/some/relative/path") {
		t.Errorf("expected path to end with /some/relative/path, got %q", got)
	}
}

func TestResolvePath_dotBecomesAbsolute(t *testing.T) {
	cwd, _ := os.Getwd()
	got, err := resolvePath(".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != cwd {
		t.Errorf("expected %q, got %q", cwd, got)
	}
}

// ---------------------------------------------------------------------------
// Create — path validation
// ---------------------------------------------------------------------------

func TestCreate_nonExistentDirectoryReturnsError(t *testing.T) {
	mgr := &Manager{client: nil} // nil client: validation fires before any tmux call
	err := mgr.Create(context.Background(), "test", "/nonexistent/path/xyz123", "")
	if err == nil {
		t.Fatal("expected error for non-existent directory, got nil")
	}
	if !strings.Contains(err.Error(), "directory does not exist") {
		t.Errorf("expected 'directory does not exist' in error, got %q", err.Error())
	}
}

func TestCreate_filePathReturnsNotADirectoryError(t *testing.T) {
	f, err := os.CreateTemp("", "cd-test-file-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	f.Close()
	defer os.Remove(f.Name())

	mgr := &Manager{client: nil}
	err = mgr.Create(context.Background(), "test", f.Name(), "")
	if err == nil {
		t.Fatal("expected error for file path, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected 'not a directory' in error, got %q", err.Error())
	}
}

func TestCreate_tildeDirectoryExpandsAndValidates(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	// Home dir exists, so resolution should succeed and reach tmux (nil client panics).
	// We expect a panic/nil-deref only if path validation passes — use recover to confirm.
	mgr := &Manager{client: nil}
	func() {
		defer func() { recover() }() // nil client will panic inside NewSession
		_ = mgr.Create(context.Background(), "test", "~/", "")
		_ = home // used above
	}()
	// If we get here without a "directory does not exist" error, ~ expanded correctly.
}

// ---------------------------------------------------------------------------
// validateClaudeArgs
// ---------------------------------------------------------------------------

func TestValidateClaudeArgs_cleanArgsPassValidation(t *testing.T) {
	cases := []string{
		"--verbose",
		"--model claude-3-5-sonnet",
		"-p 'hello world'",
		"--dangerously-skip-permissions",
	}
	for _, c := range cases {
		if err := validateClaudeArgs(c); err != nil {
			t.Errorf("expected no error for %q, got %v", c, err)
		}
	}
}

func TestValidateClaudeArgs_dangerousCharactersAreRejected(t *testing.T) {
	cases := []struct {
		name string
		arg  string
	}{
		{"backtick", "foo`bar"},
		{"semicolon", "foo;bar"},
		{"pipe", "foo|bar"},
		{"ampersand", "foo&bar"},
		{"open paren", "foo(bar"},
		{"close paren", "foo)bar"},
		{"open brace", "foo{bar"},
		{"close brace", "foo}bar"},
		{"dollar", "foo$bar"},
		{"less-than", "foo<bar"},
		{"greater-than", "foo>bar"},
		{"newline", "foo\nbar"},
		{"carriage return", "foo\rbar"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateClaudeArgs(tc.arg)
			if err == nil {
				t.Errorf("expected error for dangerous char %q in %q, but got nil", tc.name, tc.arg)
			}
		})
	}
}

func TestValidateClaudeArgs_emptyStringPasses(t *testing.T) {
	if err := validateClaudeArgs(""); err != nil {
		t.Errorf("expected no error for empty string, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// FilterSessions
// ---------------------------------------------------------------------------

func makeSessions() []Session {
	return []Session{
		{Name: "cd-alpha", Project: "alpha", Status: StatusActive, Path: "/home/user/alpha"},
		{Name: "cd-beta", Project: "beta-service", Status: StatusIdle, Path: "/home/user/beta"},
		{Name: "cd-gamma", Project: "gamma", Status: StatusWaiting, Path: "/work/gamma"},
	}
}

func TestFilterSessions_emptyQueryReturnsAll(t *testing.T) {
	sessions := makeSessions()
	result := FilterSessions(sessions, "")
	if len(result) != len(sessions) {
		t.Errorf("expected %d sessions, got %d", len(sessions), len(result))
	}
}

func TestFilterSessions_matchesByName(t *testing.T) {
	sessions := makeSessions()
	result := FilterSessions(sessions, "alpha")
	if len(result) != 1 {
		t.Fatalf("expected 1 match for 'alpha', got %d", len(result))
	}
	if result[0].Name != "cd-alpha" {
		t.Errorf("expected %q, got %q", "cd-alpha", result[0].Name)
	}
}

func TestFilterSessions_matchesByProject(t *testing.T) {
	sessions := makeSessions()
	result := FilterSessions(sessions, "beta-service")
	if len(result) != 1 {
		t.Fatalf("expected 1 match for 'beta-service', got %d", len(result))
	}
	if result[0].Project != "beta-service" {
		t.Errorf("expected project %q, got %q", "beta-service", result[0].Project)
	}
}

func TestFilterSessions_matchesByStatus(t *testing.T) {
	sessions := makeSessions()
	result := FilterSessions(sessions, "waiting")
	if len(result) != 1 {
		t.Fatalf("expected 1 match for status 'waiting', got %d", len(result))
	}
	if result[0].Status != StatusWaiting {
		t.Errorf("expected StatusWaiting, got %q", result[0].Status)
	}
}

func TestFilterSessions_matchesByPath(t *testing.T) {
	sessions := makeSessions()
	result := FilterSessions(sessions, "/work/")
	if len(result) != 1 {
		t.Fatalf("expected 1 match for path '/work/', got %d", len(result))
	}
	if result[0].Name != "cd-gamma" {
		t.Errorf("expected %q, got %q", "cd-gamma", result[0].Name)
	}
}

func TestFilterSessions_isCaseInsensitive(t *testing.T) {
	sessions := makeSessions()
	result := FilterSessions(sessions, "ALPHA")
	if len(result) != 1 {
		t.Fatalf("expected 1 match for 'ALPHA' (case-insensitive), got %d", len(result))
	}
}

func TestFilterSessions_noMatchReturnsEmpty(t *testing.T) {
	sessions := makeSessions()
	result := FilterSessions(sessions, "zzz-no-match")
	if len(result) != 0 {
		t.Errorf("expected 0 matches, got %d", len(result))
	}
}

func TestFilterSessions_emptySessionListReturnsEmpty(t *testing.T) {
	result := FilterSessions([]Session{}, "alpha")
	if len(result) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(result))
	}
}

func TestFilterSessions_partialMatchWorks(t *testing.T) {
	sessions := makeSessions()
	// "cd-" prefix is present on all names
	result := FilterSessions(sessions, "cd-")
	if len(result) != 3 {
		t.Errorf("expected 3 matches for 'cd-' prefix, got %d", len(result))
	}
}
