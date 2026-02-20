package tmux

import (
	"context"
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

// SendKeys sends keys to a tmux session.
func (c *Client) SendKeys(ctx context.Context, name, keys string) error {
	if err := validateSessionName(name); err != nil {
		return err
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.tmuxPath, "send-keys", "-t", name, keys, "Enter")
	return cmd.Run()
}

// GetSessionInfo returns detailed session info with custom format.
func (c *Client) GetSessionInfo(ctx context.Context, name, format string) (string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.tmuxPath, "display-message", "-t", name, "-p", format)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ProcEntry holds a child process PID and its command args.
type ProcEntry struct {
	PID  string
	Args string
}

// HasClaudeProcess checks if a session has a claude process in its process tree.
// procChildren maps each PID to its children ProcEntry values; pass nil to fall
// back to spawning ps (legacy path, used when no cached table is available).
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
// "claude" in its command. If procChildren is nil it falls back to spawning ps.
func hasClaudeDescendant(rootPID string, procChildren map[string][]ProcEntry) bool {
	if procChildren == nil {
		// Legacy fallback: spawn ps once.
		cmd := exec.Command("ps", "-eo", "pid,ppid,args")
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		procChildren = make(map[string][]ProcEntry)
		lines := strings.Split(string(out), "\n")
		for _, line := range lines[1:] { // skip header
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			ppid := fields[1]
			args := strings.Join(fields[2:], " ")
			procChildren[ppid] = append(procChildren[ppid], ProcEntry{PID: fields[0], Args: args})
		}
	}

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
