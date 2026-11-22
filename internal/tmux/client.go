package tmux

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const defaultTimeout = 5 * time.Second

// validSessionNameRe matches only safe tmux session name characters.
var validSessionNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// validateSessionName returns an error if name contains unsafe characters.
func validateSessionName(name string) error {
	if !validSessionNameRe.MatchString(name) {
		return fmt.Errorf("invalid session name %q: only alphanumeric, underscore, and hyphen characters are allowed", name)
	}
	return nil
}

// withTimeout returns a context with the default 5-second timeout derived from
// the parent. Callers must call the returned cancel function.
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, defaultTimeout)
}

// Client wraps tmux commands.
type Client struct {
	tmuxPath string
}

// NewClient creates a new tmux client.
func NewClient() (*Client, error) {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return nil, fmt.Errorf("tmux not found: %w", err)
	}
	return &Client{tmuxPath: path}, nil
}

// ListSessions returns raw tmux session list with format.
func (c *Client) ListSessions(ctx context.Context, format string) (string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.tmuxPath, "list-sessions", "-F", format)
	out, err := cmd.CombinedOutput()
	if err != nil {
		combined := string(out)
		if strings.Contains(combined, "no server running") ||
			strings.Contains(combined, "no current client") ||
			strings.Contains(err.Error(), "exit status") {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// NewSession creates a new tmux session.
func (c *Client) NewSession(ctx context.Context, name, startDir, command string) error {
	if err := validateSessionName(name); err != nil {
		return err
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	args := []string{"new-session", "-d", "-s", name}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	if command != "" {
		args = append(args, command)
	}
	cmd := exec.CommandContext(ctx, c.tmuxPath, args...)
	return cmd.Run()
}

// HasSession reports whether a session with exactly this name exists.
func (c *Client) HasSession(ctx context.Context, name string) bool {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	// "=" forces an exact match; a bare name also matches by prefix.
	return exec.CommandContext(ctx, c.tmuxPath, "has-session", "-t", "="+name).Run() == nil
}

// KillSession kills a tmux session by name.
func (c *Client) KillSession(ctx context.Context, name string) error {
	if err := validateSessionName(name); err != nil {
		return err
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.tmuxPath, "kill-session", "-t", name)
	return cmd.Run()
}

// MaxSendLength caps a single message so a runaway task cannot flood a pane.
const MaxSendLength = 4000

// submitDelay separates the typed text from the Enter key. Claude Code treats
// text that arrives in one burst as a paste, and an Enter inside a paste adds a
// newline instead of submitting.
const submitDelay = 300 * time.Millisecond

// ValidateSendText reports whether text can be typed into a session safely.
// It must be one non-empty line without control characters: a newline or an
// escape sequence could submit early or drive the terminal instead of typing.
func ValidateSendText(text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("message is empty")
	}
	if len(text) > MaxSendLength {
		return fmt.Errorf("message is %d bytes; the limit is %d (point to a file instead)", len(text), MaxSendLength)
	}
	for _, r := range text {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("message must be a single line without control characters (put long instructions in a file and reference its path)")
		}
	}
	return nil
}

// SendText types text into the pane where Claude Code runs and presses Enter.
// That pane is the one "claude-dashboard hook" recorded in @claude_pane, never
// the session's active pane: a split shell pane that has focus would otherwise
// run the text as shell commands. A session that has not recorded its pane is
// refused with ErrNoClaudePane.
// The text is sent literally (send-keys -l), so a message such as "C-c" is
// typed as characters and never interpreted as a key name.
func (c *Client) SendText(ctx context.Context, name, text string) error {
	if err := validateSessionName(name); err != nil {
		return err
	}
	if err := ValidateSendText(text); err != nil {
		return err
	}

	target, err := c.claudePane(ctx, name)
	if err != nil {
		return err
	}

	typeCtx, cancel := withTimeout(ctx)
	defer cancel()
	if err := exec.CommandContext(typeCtx, c.tmuxPath, "send-keys", "-t", target, "-l", "--", text).Run(); err != nil {
		return fmt.Errorf("failed to type into %s: %w", name, err)
	}

	select {
	case <-time.After(submitDelay):
	case <-ctx.Done():
		return ctx.Err()
	}

	enterCtx, cancelEnter := withTimeout(ctx)
	defer cancelEnter()
	if err := exec.CommandContext(enterCtx, c.tmuxPath, "send-keys", "-t", target, "Enter").Run(); err != nil {
		return fmt.Errorf("typed into %s but failed to press Enter (%v): %w", name, err, ErrTypedNotSubmitted)
	}
	return nil
}

// ErrTypedNotSubmitted marks a SendText failure after the text was typed: it
// is sitting in the session's input box. Retrying would type it twice, so
// callers must not resend without looking at the session first.
var ErrTypedNotSubmitted = errors.New("text typed but not submitted")

// ErrNoClaudePane means the session has not recorded which pane runs Claude
// Code, so nothing can be typed into it safely.
var ErrNoClaudePane = errors.New("session has not reported its Claude pane (update claude-dashboard and re-run setup-hooks)")

// paneIDRe matches a tmux pane id such as %3.
var paneIDRe = regexp.MustCompile(`^%[0-9]+$`)

// claudePane returns the id of the pane that runs Claude Code in the session,
// as recorded by "claude-dashboard hook" in the @claude_pane option. The id is
// validated and checked to still exist and to belong to this session, so a
// stale or tampered value can never redirect the text.
func (c *Client) claudePane(ctx context.Context, name string) (string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	// "=name:" selects the session by exact name, never by prefix.
	out, err := exec.CommandContext(ctx, c.tmuxPath, "show-options", "-qv", "-t", "="+name+":", "@claude_pane").Output()
	if err != nil {
		return "", fmt.Errorf("failed to read the Claude pane of %s: %w", name, err)
	}
	pane := strings.TrimSpace(string(out))
	if pane == "" {
		return "", fmt.Errorf("%s: %w", name, ErrNoClaudePane)
	}
	if !paneIDRe.MatchString(pane) {
		return "", fmt.Errorf("invalid pane id %q recorded for %s", pane, name)
	}

	owner, err := exec.CommandContext(ctx, c.tmuxPath, "display-message", "-p", "-t", pane, "#{session_name}").Output()
	if err != nil {
		return "", fmt.Errorf("the Claude pane %s of %s no longer exists", pane, name)
	}
	if strings.TrimSpace(string(owner)) != name {
		return "", fmt.Errorf("the recorded pane %s does not belong to session %s", pane, name)
	}
	return pane, nil
}

// CapturePaneContent captures the visible pane content of a session.
func (c *Client) CapturePaneContent(ctx context.Context, name string, historyLines int) (string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	args := []string{"capture-pane", "-t", name, "-p"}
	if historyLines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", historyLines))
	}
	cmd := exec.CommandContext(ctx, c.tmuxPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("capture-pane failed: %w", err)
	}
	return string(out), nil
}

// GetSessionPID returns the PID of the first pane's process in a session.
func (c *Client) GetSessionPID(ctx context.Context, name string) (string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.tmuxPath, "list-panes", "-t", name, "-F", "#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		return lines[0], nil
	}
	return "", fmt.Errorf("no pane found for session %s", name)
}

// ProcEntry holds a child process PID and its command args.
type ProcEntry struct {
	PID  string
	Args string
}

// HasClaudeProcess checks if a session has a claude process in its process tree.
// procChildren maps each PID to its children ProcEntry values.
func (c *Client) HasClaudeProcess(ctx context.Context, name string, procChildren map[string][]ProcEntry) bool {
	// Check pane current command first (fast path).
	tctx, cancel := withTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(tctx, c.tmuxPath, "list-panes", "-t", name, "-F", "#{pane_current_command}")
	out, err := cmd.Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.Contains(strings.ToLower(line), "claude") {
				return true
			}
		}
	}

	// Fall back to checking process tree of pane PID.
	pid, err := c.GetSessionPID(ctx, name)
	if err != nil {
		return false
	}

	return hasClaudeDescendant(pid, procChildren)
}

// BuildProcChildren converts a flat pid->ppid map into a children lookup.
// It accepts entries as (pid, ppid, args) triples represented by the caller.
// monitor.ProcessTable is not imported here to avoid cycles; callers pass the
// already-converted map.
func BuildProcChildren(entries []struct{ PID, PPID, Args string }) map[string][]ProcEntry {
	m := make(map[string][]ProcEntry)
	for _, e := range entries {
		m[e.PPID] = append(m[e.PPID], ProcEntry{PID: e.PID, Args: e.Args})
	}
	return m
}

// hasClaudeDescendant checks if any descendant process of the given PID has
// "claude" in its command.
func hasClaudeDescendant(rootPID string, procChildren map[string][]ProcEntry) bool {
	// BFS from rootPID
	queue := []string{rootPID}
	visited := make(map[string]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true

		for _, child := range procChildren[current] {
			if strings.Contains(strings.ToLower(child.Args), "claude") {
				return true
			}
			queue = append(queue, child.PID)
		}
	}

	return false
}
