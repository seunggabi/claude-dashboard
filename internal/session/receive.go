package session

import (
	"strconv"
	"strings"
	"time"
)

// SendMode controls which sessions may receive a message.
type SendMode int

const (
	// ModeIdleOnly sends only to sessions that finished their turn.
	ModeIdleOnly SendMode = iota
	// ModeQueueBehindWork also allows sessions that are mid-turn; Claude Code
	// holds the message and handles it once the current work ends.
	ModeQueueBehindWork
	// ModeReply sends only to sessions that are asking the user a question
	// (hook state "asking"), to answer it. A permission prompt is never a
	// question: Enter there can approve an action.
	ModeReply
)

// parseHook splits the "STATE:UNIXTIME" value that "claude-dashboard hook"
// stores in the @claude_state tmux option. Both results are zero when the
// value is empty or malformed.
func parseHook(value string) (string, time.Time) {
	state, at, ok := strings.Cut(value, ":")
	if !ok {
		return "", time.Time{}
	}
	ts, err := strconv.ParseInt(at, 10, 64)
	if err != nil || ts <= 0 {
		return "", time.Time{}
	}
	return state, time.Unix(ts, 0)
}

// CanReceive reports whether a message may be typed into the session now, and
// if not, why. It trusts only what Claude Code hooks reported: a state guessed
// from the screen can mistake a permission prompt for an idle prompt, and an
// Enter pressed at a permission prompt can approve the action.
//
// A session that is waiting for input or permission never receives a message.
func (s *Session) CanReceive(mode SendMode) (bool, string) {
	if !s.Managed {
		return false, "not a tmux session (read-only)"
	}

	if mode == ModeReply {
		if s.Status == StatusWaiting && s.Hook == "asking" {
			return true, ""
		}
		return false, "not asking a question"
	}

	switch s.Status {
	case StatusWaiting:
		if s.Hook == "asking" {
			return false, "asking a question (reply to it instead)"
		}
		return false, "waiting for permission or input"
	case StatusIdle:
		if s.Hook != "idle" {
			return false, "no hook report; set up Claude Code hooks (see README)"
		}
		return true, ""
	case StatusActive:
		if mode != ModeQueueBehindWork {
			return false, "busy (use queue-behind-work to allow)"
		}
		if s.Hook != "active" {
			return false, "no hook report; set up Claude Code hooks (see README)"
		}
		return true, ""
	default:
		return false, "state is " + string(s.Status)
	}
}
