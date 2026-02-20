package session

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/seunggabi/claude-dashboard/internal/monitor"
	"github.com/seunggabi/claude-dashboard/internal/tmux"
)

// cwdCacheEntry holds a cached CWD result with expiry.
type cwdCacheEntry struct {
	path    string
	expires time.Time
}

var (
	cwdCache   = make(map[string]cwdCacheEntry)
	cwdCacheMu sync.Mutex
	cwdCacheTTL = 10 * time.Second
)

// Detector discovers Claude Code sessions from tmux.
type Detector struct {
	client *tmux.Client
}

// NewDetector creates a new session detector.
func NewDetector(client *tmux.Client) *Detector {
	return &Detector{client: client}
}

// Detect finds all Claude-related tmux sessions.
func (d *Detector) Detect(ctx context.Context) ([]Session, error) {
	output, err := d.client.ListSessions(ctx, tmux.SessionFormat)
	if err != nil {
		// Even if tmux fails, still detect terminal sessions
		return d.detectTerminalOnly()
	}
	if output == "" {
		return d.detectTerminalOnly()
	}

	rawSessions := tmux.ParseSessions(output)
	sessions := make([]Session, 0, len(rawSessions))

	// Build process table and children map once for all sessions.
	procTable := monitor.GetProcessTable()
	procChildren := buildProcChildren(procTable)

	for _, raw := range rawSessions {
		// Include sessions with cd- prefix or that contain claude in the name
		isNameMatch := strings.HasPrefix(raw.Name, SessionPrefix) || strings.Contains(strings.ToLower(raw.Name), "claude")
		if !isNameMatch && !d.client.HasClaudeProcess(ctx, raw.Name, procChildren) {
			continue
		}

		s := Session{
			Name:      raw.Name,
			Project:   extractProject(raw.Name, raw.Path),
			Status:    StatusUnknown,
			StartedAt: raw.Created,
			Activity:  raw.Activity,
			Attached:  raw.Attached,
			Path:      raw.Path,
			Managed:   true,
		}

		// Detect status from pane content and activity timestamp
		s.Status = d.detectStatus(ctx, raw.Name, raw.Activity)

		// Get PID
		pid, err := d.client.GetSessionPID(ctx, raw.Name)
		if err == nil {
			s.PID = pid
		}

		sessions = append(sessions, s)
	}

	// Collect tmux session PIDs for deduplication
	tmuxPIDs := make(map[string]bool)
	for _, s := range sessions {
		if s.PID != "" {
			tmuxPIDs[s.PID] = true
		}
	}

	// Detect terminal sessions (Claude running outside tmux)
	terminalSessions := d.DetectTerminalSessions(tmuxPIDs)
	sessions = append(sessions, terminalSessions...)

	return sessions, nil
}

// detectTerminalOnly returns only terminal sessions (when tmux is unavailable).
func (d *Detector) detectTerminalOnly() ([]Session, error) {
	sessions := d.DetectTerminalSessions(make(map[string]bool))
	return sessions, nil
}

// DetectTerminalSessions finds Claude processes running outside tmux.
func (d *Detector) DetectTerminalSessions(tmuxPIDs map[string]bool) []Session {
	cmd := exec.Command("ps", "-eo", "pid,ppid,tty,args")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var sessions []Session
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid := fields[0]
		tty := fields[2]

		// Only match processes where the executable base name is "claude"
		execName := fields[3]
		parts := strings.Split(execName, "/")
		baseName := parts[len(parts)-1]
		if baseName != "claude" {
			continue
		}

		// Skip if already tracked as tmux session
		if tmuxPIDs[pid] {
			continue
		}

		// Skip background/detached processes (no TTY)
		if tty == "??" || tty == "?" {
			continue
		}

		// Get working directory via lsof
		path := getProcessCWD(pid)

		project := ""
		if path != "" {
			pathParts := strings.Split(strings.TrimRight(path, "/"), "/")
			if len(pathParts) > 0 {
				project = pathParts[len(pathParts)-1]
			}
		}

		s := Session{
			Name:    fmt.Sprintf("terminal/%s", tty),
			Project: project,
			Status:  StatusTerminal,
			PID:     pid,
			Path:    path,
			Managed: false,
		}
		sessions = append(sessions, s)
	}

	return sessions
}

// getProcessCWD gets the current working directory of a process.
// Results are cached with a 10-second TTL to avoid repeated lsof calls.
func getProcessCWD(pid string) string {
	cwdCacheMu.Lock()
	if entry, ok := cwdCache[pid]; ok && time.Now().Before(entry.expires) {
		cwdCacheMu.Unlock()
		return entry.path
	}
	cwdCacheMu.Unlock()

	cmd := exec.Command("lsof", "-a", "-p", pid, "-d", "cwd", "-Fn")
	out, err := cmd.Output()
	result := ""
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "n/") {
				result = line[1:]
				break
			}
		}
	}

	cwdCacheMu.Lock()
	cwdCache[pid] = cwdCacheEntry{path: result, expires: time.Now().Add(cwdCacheTTL)}
	cwdCacheMu.Unlock()

	return result
}

// detectStatus determines session status by examining activity timestamp and pane content.
func (d *Detector) detectStatus(ctx context.Context, name string, lastActivity time.Time) Status {
	// If activity is very recent (within 2 seconds), consider it active
	// This handles cases where output is streaming but prompt is not visible yet
	idleThreshold := 2 * time.Second
	if !lastActivity.IsZero() && time.Since(lastActivity) < idleThreshold {
		return StatusActive
	}

	// If no recent activity, check pane content to distinguish idle vs waiting
	content, err := d.client.CapturePaneContent(ctx, name, 20)
	if err != nil {
		return StatusIdle
	}

	lines := strings.Split(content, "\n")
	hasPrompt := false

	// Check last 20 lines for status indicators
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-20; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Waiting for input (confirmation prompts)
		// Match "?" only at end of line, or known confirmation patterns.
		endsWithQuestion := strings.HasSuffix(line, "?")
		hasConfirmPattern := strings.Contains(line, "(y/n)") || strings.Contains(line, "(Y/n)") ||
			strings.Contains(line, "(y/N)") || strings.Contains(line, "Y/n") || strings.Contains(line, "y/N")
		if endsWithQuestion || hasConfirmPattern {
			return StatusWaiting
		}

		// Prompt visible = idle
		// Match "$" only at end of line to avoid false positives on shell variables.
		endsWithDollar := strings.HasSuffix(line, "$")
		if strings.HasPrefix(line, ">") || strings.Contains(line, "❯") || endsWithDollar {
			hasPrompt = true
		}
	}

	// If prompt found, it's idle. Otherwise, unknown.
	if hasPrompt {
		return StatusIdle
	}

	return StatusIdle // Default to idle when no recent activity
}

// buildProcChildren converts a monitor.ProcessTable into the children map
// format expected by tmux.Client.HasClaudeProcess.
func buildProcChildren(table monitor.ProcessTable) map[string][]tmux.ProcEntry {
	entries := make([]struct{ PID, PPID, Args string }, 0, len(table))
	for _, e := range table {
		entries = append(entries, struct{ PID, PPID, Args string }{
			PID:  e.PID,
			PPID: e.PPID,
			Args: e.Args,
		})
	}
	return tmux.BuildProcChildren(entries)
}

// extractProject derives project name from session name or path.
func extractProject(name, path string) string {
	// If session has cd- prefix, use the rest as project name
	if strings.HasPrefix(name, SessionPrefix) {
		return strings.TrimPrefix(name, SessionPrefix)
	}

	// Use last directory component of path
	if path != "" {
		parts := strings.Split(strings.TrimRight(path, "/"), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	return name
}
