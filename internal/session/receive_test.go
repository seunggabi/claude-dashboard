package session

import (
	"testing"
	"time"
)

func TestParseHook(t *testing.T) {
	cases := []struct {
		in    string
		state string
		ok    bool
	}{
		{"idle:1700000000", "idle", true},
		{"waiting:1700000001", "waiting", true},
		{"", "", false},
		{"idle", "", false},
		{"idle:abc", "", false},
		{"idle:0", "", false},
		{"idle:-5", "", false},
	}
	for _, c := range cases {
		state, at := parseHook(c.in)
		if state != c.state || at.IsZero() == c.ok {
			t.Errorf("parseHook(%q) = %q, %v; want %q (ok=%v)", c.in, state, at, c.state, c.ok)
		}
	}
	if _, at := parseHook("idle:1700000000"); !at.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("wrong timestamp: %v", at)
	}
}

func TestCanReceive(t *testing.T) {
	cases := []struct {
		name    string
		s       Session
		mode    SendMode
		allowed bool
	}{
		{"idle with hook", Session{Managed: true, Status: StatusIdle, Hook: "idle"}, ModeIdleOnly, true},
		{"idle with hook, queue mode", Session{Managed: true, Status: StatusIdle, Hook: "idle"}, ModeQueueBehindWork, true},
		{"idle guessed from screen (no hook)", Session{Managed: true, Status: StatusIdle}, ModeIdleOnly, false},
		{"idle but hook says active (stale)", Session{Managed: true, Status: StatusIdle, Hook: "active"}, ModeIdleOnly, false},
		{"idle but hook says waiting (stale)", Session{Managed: true, Status: StatusIdle, Hook: "waiting"}, ModeQueueBehindWork, false},
		{"active, idle-only mode", Session{Managed: true, Status: StatusActive, Hook: "active"}, ModeIdleOnly, false},
		{"active, queue mode", Session{Managed: true, Status: StatusActive, Hook: "active"}, ModeQueueBehindWork, true},
		{"active guessed (no hook), queue mode", Session{Managed: true, Status: StatusActive}, ModeQueueBehindWork, false},
		{"waiting never, idle-only", Session{Managed: true, Status: StatusWaiting, Hook: "waiting"}, ModeIdleOnly, false},
		{"waiting never, queue mode", Session{Managed: true, Status: StatusWaiting, Hook: "waiting"}, ModeQueueBehindWork, false},
		{"waiting guessed from screen, queue mode", Session{Managed: true, Status: StatusWaiting}, ModeQueueBehindWork, false},
		{"asking a question, reply mode", Session{Managed: true, Status: StatusWaiting, Hook: "asking"}, ModeReply, true},
		{"asking a question, idle-only mode", Session{Managed: true, Status: StatusWaiting, Hook: "asking"}, ModeIdleOnly, false},
		{"asking a question, queue mode", Session{Managed: true, Status: StatusWaiting, Hook: "asking"}, ModeQueueBehindWork, false},
		{"permission prompt, reply mode", Session{Managed: true, Status: StatusWaiting, Hook: "waiting"}, ModeReply, false},
		{"waiting guessed from screen, reply mode", Session{Managed: true, Status: StatusWaiting}, ModeReply, false},
		{"idle, reply mode", Session{Managed: true, Status: StatusIdle, Hook: "idle"}, ModeReply, false},
		{"active, reply mode", Session{Managed: true, Status: StatusActive, Hook: "active"}, ModeReply, false},
		{"unmanaged asking, reply mode", Session{Managed: false, Status: StatusWaiting, Hook: "asking"}, ModeReply, false},
		{"terminal process", Session{Managed: false, Status: StatusTerminal}, ModeQueueBehindWork, false},
		{"unmanaged even if idle with hook", Session{Managed: false, Status: StatusIdle, Hook: "idle"}, ModeIdleOnly, false},
		{"unknown status", Session{Managed: true, Status: StatusUnknown, Hook: "idle"}, ModeQueueBehindWork, false},
	}
	for _, c := range cases {
		ok, reason := c.s.CanReceive(c.mode)
		if ok != c.allowed {
			t.Errorf("%s: got %v (%q), want %v", c.name, ok, reason, c.allowed)
		}
		if !ok && reason == "" {
			t.Errorf("%s: a refusal must explain why", c.name)
		}
	}
}
