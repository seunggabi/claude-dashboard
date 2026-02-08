package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

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

// IsRunning checks if tmux server is running.
func (c *Client) IsRunning() bool {
	cmd := exec.Command(c.tmuxPath, "list-sessions")
	err := cmd.Run()
	return err == nil
}

// ListSessions returns raw tmux session list with format.
func (c *Client) ListSessions(format string) (string, error) {
	cmd := exec.Command(c.tmuxPath, "list-sessions", "-F", format)
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
func (c *Client) NewSession(name, startDir, command string) error {
	args := []string{"new-session", "-d", "-s", name}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	if command != "" {
		args = append(args, command)
	}
	cmd := exec.Command(c.tmuxPath, args...)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Enable mouse scrolling for this session
	mouseCmd := exec.Command(c.tmuxPath, "set-option", "-t", name, "mouse", "on")
	_ = mouseCmd.Run()

	return nil
}

// KillSession kills a tmux session by name.
func (c *Client) KillSession(name string) error {
	cmd := exec.Command(c.tmuxPath, "kill-session", "-t", name)
	return cmd.Run()
}

// AttachSession returns the exec.Cmd to attach to a session.
func (c *Client) AttachSession(name string) *exec.Cmd {
	return exec.Command(c.tmuxPath, "attach-session", "-t", name)
}

// CapturePaneContent captures the visible pane content of a session.
func (c *Client) CapturePaneContent(name string, historyLines int) (string, error) {
	args := []string{"capture-pane", "-t", name, "-p"}
	if historyLines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", historyLines))
	}
	cmd := exec.Command(c.tmuxPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("capture-pane failed: %w", err)
	}
	return string(out), nil
}

// GetSessionPID returns the PID of the first pane's process in a session.
func (c *Client) GetSessionPID(name string) (string, error) {
	cmd := exec.Command(c.tmuxPath, "list-panes", "-t", name, "-F", "#{pane_pid}")
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
func (c *Client) SendKeys(name, keys string) error {
	cmd := exec.Command(c.tmuxPath, "send-keys", "-t", name, keys, "Enter")
	return cmd.Run()
}

// GetSessionInfo returns detailed session info with custom format.
func (c *Client) GetSessionInfo(name, format string) (string, error) {
	cmd := exec.Command(c.tmuxPath, "display-message", "-t", name, "-p", format)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// HasClaudeProcess checks if a session has a claude process in its process tree.
func (c *Client) HasClaudeProcess(name string) bool {
	// Check pane current command first (fast path)
	cmd := exec.Command(c.tmuxPath, "list-panes", "-t", name, "-F", "#{pane_current_command}")
	out, err := cmd.Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.Contains(strings.ToLower(line), "claude") {
				return true
			}
		}
	}

	// Fall back to checking process tree of pane PID
	pid, err := c.GetSessionPID(name)
	if err != nil {
		return false
	}

	return hasClaudeDescendant(pid)
}

// hasClaudeDescendant checks if any descendant process of the given PID has "claude" in its command.
func hasClaudeDescendant(rootPID string) bool {
	cmd := exec.Command("ps", "-eo", "pid,ppid,args")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	type proc struct {
		pid  string
		args string
	}

	childrenMap := make(map[string][]proc)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid := fields[0]
		ppid := fields[1]
		args := strings.Join(fields[2:], " ")
		childrenMap[ppid] = append(childrenMap[ppid], proc{pid: pid, args: args})
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

		for _, child := range childrenMap[current] {
			if strings.Contains(strings.ToLower(child.args), "claude") {
				return true
			}
			queue = append(queue, child.pid)
		}
	}

	return false
}
