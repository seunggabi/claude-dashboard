package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withHome points $HOME at a temp dir so tests never touch the real one.
func withHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func backups(t *testing.T, home string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(home, ".tmux.conf.claude-dashboard-bak-*"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestBuildTmuxConfigKeepsUserLines(t *testing.T) {
	existing := "set -g prefix C-a\nbind r source-file ~/.tmux.conf\n"
	got := buildTmuxConfig(existing)

	if !strings.HasPrefix(got, "set -g prefix C-a\nbind r source-file ~/.tmux.conf\n") {
		t.Fatalf("user lines were not preserved:\n%s", got)
	}
	if !strings.HasSuffix(got, tmuxConfigBlock) {
		t.Fatalf("claude-dashboard block was not appended:\n%s", got)
	}
}

func TestBuildTmuxConfigIsIdempotent(t *testing.T) {
	once := buildTmuxConfig("set -g prefix C-a\n")
	twice := buildTmuxConfig(once)
	if once != twice {
		t.Fatalf("applying twice changed the result:\nonce:\n%s\ntwice:\n%s", once, twice)
	}
	if n := strings.Count(twice, "# claude-dashboard: F12"); n != 1 {
		t.Fatalf("expected one F12 block, got %d", n)
	}
}

func TestBuildTmuxConfigReplacesOldBlock(t *testing.T) {
	existing := "set -g prefix C-a\n\n" +
		"# claude-dashboard: F12 key binding for mouse mode toggle\n" +
		"bind-key -n F12 run-shell \"~/.local/bin/claude-dashboard-mouse-toggle\"\n"
	got := buildTmuxConfig(existing)

	if strings.Count(got, "claude-dashboard-mouse-toggle") != 1 {
		t.Fatalf("old block was not replaced:\n%s", got)
	}
	if !strings.Contains(got, "set -g prefix C-a") {
		t.Fatalf("user line lost:\n%s", got)
	}
}

func TestSetupTmuxConfigBacksUpExistingFile(t *testing.T) {
	home := withHome(t)
	conf := filepath.Join(home, ".tmux.conf")
	original := "set -g prefix C-a\nset -g status-right 'mine'\n"
	if err := os.WriteFile(conf, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	backup, err := SetupTmuxConfigWithBackup()
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("expected a backup path")
	}
	saved, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != original {
		t.Fatalf("backup differs from original:\n%s", saved)
	}
	updated, _ := os.ReadFile(conf)
	if !strings.Contains(string(updated), "claude-dashboard") {
		t.Fatalf("tmux.conf was not updated:\n%s", updated)
	}
}

func TestSetupTmuxConfigNoBackupWhenFileMissing(t *testing.T) {
	home := withHome(t)

	backup, err := SetupTmuxConfigWithBackup()
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" || len(backups(t, home)) != 0 {
		t.Fatalf("no backup expected, got %q", backup)
	}
	if _, err := os.Stat(filepath.Join(home, ".tmux.conf")); err != nil {
		t.Fatalf("tmux.conf should have been created: %v", err)
	}
}

func TestSetupTmuxConfigSkipsWhenUpToDate(t *testing.T) {
	home := withHome(t)
	if _, err := SetupTmuxConfigWithBackup(); err != nil {
		t.Fatal(err)
	}

	backup, err := SetupTmuxConfigWithBackup()
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" || len(backups(t, home)) != 0 {
		t.Fatalf("second run must not create a backup, got %q", backup)
	}
}

func TestDeclinedMarkerRoundTrip(t *testing.T) {
	withHome(t)

	if IsDeclined() {
		t.Fatal("fresh home must not be declined")
	}
	if err := MarkDeclined(); err != nil {
		t.Fatal(err)
	}
	if !IsDeclined() {
		t.Fatal("expected declined after MarkDeclined")
	}
	if err := ClearDeclined(); err != nil {
		t.Fatal(err)
	}
	if IsDeclined() {
		t.Fatal("expected not declined after ClearDeclined")
	}
	if err := ClearDeclined(); err != nil {
		t.Fatalf("clearing twice must not fail: %v", err)
	}
}

// A line written directly under an old claude-dashboard marker comment, with
// no blank line between them, is treated as part of that block and replaced.
// This pins the behaviour: it is why setup always backs up ~/.tmux.conf first.
func TestBuildTmuxConfigDropsLinesGluedToOldMarker(t *testing.T) {
	existing := "# claude-dashboard: Enable mouse mode by default\n" +
		"set -g mouse on\n" +
		"set -g default-terminal \"screen-256color\"\n" +
		"\n" +
		"set -g prefix C-a\n"
	got := buildTmuxConfig(existing)

	if strings.Contains(got, "screen-256color") {
		t.Fatalf("expected the glued line to be replaced (backup covers it); behaviour changed:\n%s", got)
	}
	if !strings.Contains(got, "set -g prefix C-a") {
		t.Fatalf("a line after the blank separator must be kept:\n%s", got)
	}
}

// Users who already ran the old setup have the helper scripts installed.
// CheckSetup must report them as done no matter what their tmux.conf holds,
// otherwise they would see the new first-run prompt on every launch.
func TestCheckSetupReflectsInstalledScripts(t *testing.T) {
	home := withHome(t)

	if CheckSetup() {
		t.Fatal("a fresh home must need setup")
	}
	if err := InstallScripts(); err != nil {
		t.Fatal(err)
	}
	if !CheckSetup() {
		t.Fatal("scripts installed: setup must count as done")
	}

	custom := "set -g prefix C-a\nset -g status-right 'mine'\n"
	if err := os.WriteFile(filepath.Join(home, ".tmux.conf"), []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}
	if !CheckSetup() {
		t.Fatal("a customised tmux.conf must not make an existing user look un-set-up")
	}
}

func TestPlanListsEveryChange(t *testing.T) {
	plan := Plan()
	for _, s := range helperScripts {
		if !strings.Contains(plan, s.name) {
			t.Errorf("plan does not mention script %s", s.name)
		}
	}
	for _, want := range []string{"~/.tmux.conf", "Back up", "status-right", "mouse on"} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan does not mention %q", want)
		}
	}
}
