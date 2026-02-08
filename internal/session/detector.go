package session

import (
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
		return nil, err
	}
	if output == "" {
		return nil, nil
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

	return sessions, nil
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
