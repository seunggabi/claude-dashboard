package tmux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestValidateSendText(t *testing.T) {
	bad := map[string]string{
		"empty":    "",
		"blank":    "   ",
		"newline":  "line one\nline two",
		"carriage": "run\rit",
		"escape":   "hello\x1b[31m",
		"tab":      "a\tb",
		"delete":   "a\x7fb",
		"too long": strings.Repeat("x", MaxSendLength+1),
	}
	for name, text := range bad {
		if err := ValidateSendText(text); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	good := []string{"fix the failing test", "C-c", "한글 작업 지시", strings.Repeat("x", MaxSendLength)}
	for _, text := range good {
		if err := ValidateSendText(text); err != nil {
			t.Errorf("%q: unexpected error %v", text, err)
		}
	}
}

// isolatedTmux starts a private tmux server (its own socket dir) so the test
// can never touch the user's real sessions.
func isolatedTmux(t *testing.T) *Client {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	// Unix socket paths are short on macOS, so avoid the long t.TempDir path.
	dir, err := os.MkdirTemp("/tmp", "cdt")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-server").Run()
		_ = os.RemoveAll(dir)
	})
	c, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// startCat starts a session running cat and records its pane in @claude_pane
// the way "claude-dashboard hook" does. It returns the pane id.
func startCat(t *testing.T, name string) string {
	t.Helper()
	pane := startCatUnrecorded(t, name)
	if out, err := exec.Command("tmux", "set-option", "-q", "-t", pane, "@claude_pane", pane).CombinedOutput(); err != nil {
		t.Fatalf("set @claude_pane on %s: %v\n%s", name, err, out)
	}
	return pane
}

// startCatUnrecorded starts a session running cat without recording its pane.
func startCatUnrecorded(t *testing.T, name string) string {
	t.Helper()
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-x", "120", "-y", "30", "cat").CombinedOutput(); err != nil {
		t.Fatalf("new-session %s: %v\n%s", name, err, out)
	}
	out, err := exec.Command("tmux", "display-message", "-p", "-t", "="+name+":", "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("pane id of %s: %v", name, err)
	}
	return strings.TrimSpace(string(out))
}

func paneText(t *testing.T, c *Client, name string) string {
	t.Helper()
	out, err := c.CapturePaneContent(context.Background(), name, 50)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSendTextTypesLiterallyAndSubmits(t *testing.T) {
	c := isolatedTmux(t)
	startCat(t, "cdtest")

	// "C-c" must be typed as characters, not delivered as Ctrl-C (which would kill cat).
	if err := c.SendText(context.Background(), "cdtest", "C-c hello"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	// cat echoes the typed line once and prints it again after Enter.
	got := paneText(t, c, "cdtest")
	if n := strings.Count(got, "C-c hello"); n < 2 {
		t.Fatalf("expected the line typed and submitted (>=2 occurrences), got %d:\n%s", n, got)
	}
	if !c.HasSession(context.Background(), "cdtest") {
		t.Fatal("session died: C-c was interpreted as a key")
	}
}

func TestSendTextUsesExactSessionName(t *testing.T) {
	c := isolatedTmux(t)
	startCat(t, "cdtest")

	// "cdtes" is a prefix of "cdtest"; it must not match.
	if err := c.SendText(context.Background(), "cdtes", "should not arrive"); err == nil {
		t.Fatal("expected an error for a name that only matches by prefix")
	}
	time.Sleep(200 * time.Millisecond)
	if got := paneText(t, c, "cdtest"); strings.Contains(got, "should not arrive") {
		t.Fatalf("message leaked to a prefix-matched session:\n%s", got)
	}
}

func TestSendTextRejectsBadInputBeforeTyping(t *testing.T) {
	c := isolatedTmux(t)
	startCat(t, "cdtest")

	for _, text := range []string{"a\nb", ""} {
		if err := c.SendText(context.Background(), "cdtest", text); err == nil {
			t.Errorf("expected rejection for %q", text)
		}
	}
	if err := c.SendText(context.Background(), "bad name;rm", "x"); err == nil {
		t.Error("expected rejection for an unsafe session name")
	}
	time.Sleep(200 * time.Millisecond)
	if got := strings.TrimSpace(paneText(t, c, "cdtest")); got != "" {
		t.Fatalf("nothing should have been typed, pane has:\n%s", got)
	}
}

func capturePane(t *testing.T, pane string) string {
	t.Helper()
	out, err := exec.Command("tmux", "capture-pane", "-p", "-t", pane).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// A focused split pane must never receive the text: only the recorded Claude pane.
func TestSendTextTypesIntoRecordedPaneNotActivePane(t *testing.T) {
	c := isolatedTmux(t)
	claude := startCat(t, "cdtest")

	// split-window moves focus to the new pane.
	out, err := exec.Command("tmux", "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", claude, "cat").Output()
	if err != nil {
		t.Fatal(err)
	}
	other := strings.TrimSpace(string(out))
	if out, err := exec.Command("tmux", "select-pane", "-t", other).CombinedOutput(); err != nil {
		t.Fatalf("select-pane: %v\n%s", err, out)
	}

	if err := c.SendText(context.Background(), "cdtest", "only for claude"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	if got := capturePane(t, claude); !strings.Contains(got, "only for claude") {
		t.Fatalf("text did not reach the recorded pane:\n%s", got)
	}
	if got := capturePane(t, other); strings.Contains(got, "only for claude") {
		t.Fatalf("text leaked into the focused split pane:\n%s", got)
	}
}

func TestSendTextRefusesSessionWithoutRecordedPane(t *testing.T) {
	c := isolatedTmux(t)
	pane := startCatUnrecorded(t, "cdtest")

	err := c.SendText(context.Background(), "cdtest", "hello")
	if !errors.Is(err, ErrNoClaudePane) {
		t.Fatalf("err = %v, want ErrNoClaudePane", err)
	}
	time.Sleep(200 * time.Millisecond)
	if got := strings.TrimSpace(capturePane(t, pane)); got != "" {
		t.Fatalf("nothing should have been typed, pane has:\n%s", got)
	}
}

func TestSendTextRefusesMalformedPaneValue(t *testing.T) {
	c := isolatedTmux(t)
	pane := startCatUnrecorded(t, "cdtest")
	if out, err := exec.Command("tmux", "set-option", "-q", "-t", pane, "@claude_pane", "%1;kill-server").CombinedOutput(); err != nil {
		t.Fatalf("set option: %v\n%s", err, out)
	}

	if err := c.SendText(context.Background(), "cdtest", "hello"); err == nil {
		t.Fatal("expected a malformed pane id to be refused")
	}
	if !c.HasSession(context.Background(), "cdtest") {
		t.Fatal("tmux server was affected by a malformed pane value")
	}
	if got := strings.TrimSpace(capturePane(t, pane)); got != "" {
		t.Fatalf("nothing should have been typed, pane has:\n%s", got)
	}
}

func TestSendTextRefusesPaneOfAnotherSession(t *testing.T) {
	c := isolatedTmux(t)
	startCatUnrecorded(t, "cdone")
	victim := startCat(t, "cdtwo")

	// cdone claims cdtwo's pane as its own.
	if out, err := exec.Command("tmux", "set-option", "-q", "-t", "=cdone:", "@claude_pane", victim).CombinedOutput(); err != nil {
		t.Fatalf("set option: %v\n%s", err, out)
	}

	if err := c.SendText(context.Background(), "cdone", "hello"); err == nil {
		t.Fatal("expected a pane of another session to be refused")
	}
	time.Sleep(200 * time.Millisecond)
	if got := strings.TrimSpace(capturePane(t, victim)); got != "" {
		t.Fatalf("text leaked into another session's pane:\n%s", got)
	}
}
