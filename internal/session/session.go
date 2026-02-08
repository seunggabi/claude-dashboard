package session

import (
	"fmt"
	"time"
)

// Status represents the session state.
type Status string

const (
	StatusActive  Status = "active"
	StatusIdle    Status = "idle"
	StatusWaiting Status = "waiting"
	StatusUnknown  Status = "unknown"
	StatusTerminal Status = "terminal"
)

// Session represents a Claude Code tmux session.
type Session struct {
	Name      string
	Project   string
	Status    Status
	StartedAt time.Time
	Attached  bool
	PID       string
	CPU       float64
	Memory    float64
	Path      string
	Managed   bool // true = tmux session (can attach/detach), false = terminal process (read-only)
}

// Uptime returns the human-readable uptime string.
func (s *Session) Uptime() string {
	d := time.Since(s.StartedAt)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}

// StatusString returns a colored status string.
func (s *Session) StatusString() string {
	switch s.Status {
	case StatusActive:
		return "● active"
	case StatusIdle:
		return "○ idle"
	case StatusWaiting:
		return "◎ waiting"
	case StatusTerminal:
		return "⊘ terminal"
	default:
		return "? unknown"
	}
}

// DisplayName returns the display name without the cd- prefix.
func (s *Session) DisplayName() string {
	return s.Name
}

// SessionPrefix is the prefix for claude-dashboard managed sessions.
const SessionPrefix = "cd-"
