package tmux

import (
	"strconv"
	"strings"
	"time"
)

// RawSession holds parsed tmux session data.
type RawSession struct {
	Name     string
	Created  time.Time
	Attached bool
	Windows  int
	Activity time.Time
	Path     string
	// ClaudeState is the @claude_state session option set by the
	// claude-dashboard hook command, or "" when hooks are not installed.
	ClaudeState string
}

// SessionFormat is the tmux format string for listing sessions.
// The activity field is window_activity: it follows pane output, whereas
// session_activity only follows client input and stays still while detached.
const SessionFormat = "#{session_name}|#{session_created}|#{session_attached}|#{session_windows}|#{window_activity}|#{@claude_state}|#{session_path}"

// ParseSessions parses tmux list-sessions output.
func ParseSessions(output string) []RawSession {
	if output == "" {
		return nil
	}

	lines := strings.Split(output, "\n")
	sessions := make([]RawSession, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}

		created := parseUnixTimestamp(parts[1])
		attached := parts[2] == "1"
		windows, _ := strconv.Atoi(parts[3])
		activity := parseUnixTimestamp(parts[4])

		raw := RawSession{
			Name:     parts[0],
			Created:  created,
			Attached: attached,
			Windows:  windows,
			Activity: activity,
			Path:     parts[5],
		}
		// Path comes last and may itself contain "|".
		if len(parts) > 6 {
			raw.ClaudeState = parts[5]
			raw.Path = strings.Join(parts[6:], "|")
		}
		sessions = append(sessions, raw)
	}

	return sessions
}

func parseUnixTimestamp(s string) time.Time {
	ts, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(ts, 0)
}
