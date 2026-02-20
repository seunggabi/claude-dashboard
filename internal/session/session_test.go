package session

import (
	"testing"
	"time"
)

// newSessionWithAge creates a Session whose StartedAt is age ago from now.
func newSessionWithAge(age time.Duration) *Session {
	return &Session{StartedAt: time.Now().Add(-age)}
}

func TestUptime_lessThanOneMinuteShowsSeconds(t *testing.T) {
	s := newSessionWithAge(45 * time.Second)
	uptime := s.Uptime()
	// Should be something like "45s"
	if len(uptime) == 0 {
		t.Fatal("expected non-empty uptime")
	}
	if uptime[len(uptime)-1] != 's' {
		t.Errorf("expected uptime to end with 's' for <1m duration, got %q", uptime)
	}
}

func TestUptime_lessThanOneHourShowsMinutes(t *testing.T) {
	s := newSessionWithAge(30 * time.Minute)
	uptime := s.Uptime()
	if uptime[len(uptime)-1] != 'm' {
		t.Errorf("expected uptime to end with 'm' for <1h duration, got %q", uptime)
	}
}

func TestUptime_lessThanOneDayShowsHoursAndMinutes(t *testing.T) {
	s := newSessionWithAge(2*time.Hour + 30*time.Minute)
	uptime := s.Uptime()
	// Expected format: "2h30m"
	if uptime != "2h30m" {
		t.Errorf("expected %q, got %q", "2h30m", uptime)
	}
}

func TestUptime_moreThanOneDayShowsDaysAndHours(t *testing.T) {
	s := newSessionWithAge(25 * time.Hour)
	uptime := s.Uptime()
	// Expected format: "1d1h"
	if uptime != "1d1h" {
		t.Errorf("expected %q, got %q", "1d1h", uptime)
	}
}

func TestUptime_exactlyOneMinute(t *testing.T) {
	s := newSessionWithAge(time.Minute)
	uptime := s.Uptime()
	if uptime[len(uptime)-1] != 'm' {
		t.Errorf("expected uptime to end with 'm' for exactly 1m, got %q", uptime)
	}
}

func TestUptime_exactlyOneHour(t *testing.T) {
	s := newSessionWithAge(time.Hour)
	uptime := s.Uptime()
	if uptime != "1h0m" {
		t.Errorf("expected %q, got %q", "1h0m", uptime)
	}
}

func TestUptime_multiDaySession(t *testing.T) {
	s := newSessionWithAge(48 * time.Hour)
	uptime := s.Uptime()
	// 48h = 2d0h
	if uptime != "2d0h" {
		t.Errorf("expected %q, got %q", "2d0h", uptime)
	}
}

func TestStatusString_activeStatus(t *testing.T) {
	s := &Session{Status: StatusActive}
	if s.StatusString() != "● active" {
		t.Errorf("expected %q, got %q", "● active", s.StatusString())
	}
}

func TestStatusString_idleStatus(t *testing.T) {
	s := &Session{Status: StatusIdle}
	if s.StatusString() != "○ idle" {
		t.Errorf("expected %q, got %q", "○ idle", s.StatusString())
	}
}

func TestStatusString_waitingStatus(t *testing.T) {
	s := &Session{Status: StatusWaiting}
	if s.StatusString() != "◎ waiting" {
		t.Errorf("expected %q, got %q", "◎ waiting", s.StatusString())
	}
}

func TestStatusString_terminalStatus(t *testing.T) {
	s := &Session{Status: StatusTerminal}
	if s.StatusString() != "⊘ terminal" {
		t.Errorf("expected %q, got %q", "⊘ terminal", s.StatusString())
	}
}

func TestStatusString_unknownStatusFallback(t *testing.T) {
	s := &Session{Status: StatusUnknown}
	if s.StatusString() != "? unknown" {
		t.Errorf("expected %q, got %q", "? unknown", s.StatusString())
	}
}

func TestStatusString_unrecognisedStatusFallback(t *testing.T) {
	s := &Session{Status: Status("bogus")}
	if s.StatusString() != "? unknown" {
		t.Errorf("expected default fallback %q, got %q", "? unknown", s.StatusString())
	}
}

func TestDisplayName_stripsPrefix(t *testing.T) {
	s := &Session{Name: "cd-myproject"}
	if s.DisplayName() != "myproject" {
		t.Errorf("expected %q, got %q", "myproject", s.DisplayName())
	}
}

func TestDisplayName_noPrefix(t *testing.T) {
	s := &Session{Name: "myproject"}
	if s.DisplayName() != "myproject" {
		t.Errorf("expected %q, got %q", "myproject", s.DisplayName())
	}
}

func TestSessionPrefix_value(t *testing.T) {
	if SessionPrefix != "cd-" {
		t.Errorf("expected SessionPrefix to be %q, got %q", "cd-", SessionPrefix)
	}
}
