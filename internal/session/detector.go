package session

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/seunggabi/claude-dashboard/internal/tmux"
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
func (d *Detector) Detect() ([]Session, error) {
	output, err := d.client.ListSessions(tmux.SessionFormat)
	if err != nil {
		// Even if tmux fails, still detect terminal sessions
		return d.detectTerminalOnly()
	}
	if output == "" {
		return d.detectTerminalOnly()
	}

	rawSessions := tmux.ParseSessions(output)
	sessions := make([]Session, 0, len(rawSessions))

	for _, raw := range rawSessions {
		// Include sessions with cd- prefix or that contain claude in the name
		isNameMatch := strings.HasPrefix(raw.Name, SessionPrefix) || strings.Contains(strings.ToLower(raw.Name), "claude")
		if !isNameMatch && !d.client.HasClaudeProcess(raw.Name) {
			continue
		}

		s := Session{
			Name:      raw.Name,
			Project:   extractProject(raw.Name, raw.Path),
			Status:    StatusUnknown,
			StartedAt: raw.Created,
			Attached:  raw.Attached,
			Path:      raw.Path,
			Managed:   true,
		}

		// Detect status from pane content
		s.Status = d.detectStatus(raw.Name)

		// Get PID
		pid, err := d.client.GetSessionPID(raw.Name)
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
func getProcessCWD(pid string) string {
	cmd := exec.Command("lsof", "-a", "-p", pid, "-d", "cwd", "-Fn")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n/") {
			return line[1:]
		}
	}
	return ""
}

// detectStatus determines session status by examining pane content.
func (d *Detector) detectStatus(name string) Status {
	content, err := d.client.CapturePaneContent(name, 5)
	if err != nil {
		return StatusUnknown
	}

	lines := strings.Split(content, "\n")
	// Check last non-empty lines
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-5; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// Waiting for input
		if strings.Contains(line, "?") || strings.Contains(line, "Y/n") || strings.Contains(line, "y/N") {
			return StatusWaiting
		}
		// Prompt visible = idle
		if strings.HasPrefix(line, ">") || strings.Contains(line, "❯") || strings.Contains(line, "$") {
			return StatusIdle
		}
		// Otherwise likely active
		return StatusActive
	}

	return StatusIdle
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
