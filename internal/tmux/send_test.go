package tmux

import (
	"context"
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

func startCat(t *testing.T, name string) {
	t.Helper()
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-x", "120", "-y", "30", "cat").CombinedOutput(); err != nil {
		t.Fatalf("new-session %s: %v\n%s", name, err, out)
	}
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
