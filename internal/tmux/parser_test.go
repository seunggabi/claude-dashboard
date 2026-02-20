package tmux

import (
	"testing"
	"time"
)

func TestParseSessions_emptyInputReturnsNil(t *testing.T) {
	result := ParseSessions("")
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseSessions_singleValidLine(t *testing.T) {
	// Unix timestamp 1700000000 = 2023-11-14 22:13:20 UTC
	input := "my-session|1700000000|1|3|1700000100|/home/user/project"
	sessions := ParseSessions(input)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.Name != "my-session" {
		t.Errorf("Name: expected %q, got %q", "my-session", s.Name)
	}
	if !s.Attached {
		t.Error("expected Attached=true")
	}
	if s.Windows != 3 {
		t.Errorf("Windows: expected 3, got %d", s.Windows)
	}
	if s.Path != "/home/user/project" {
		t.Errorf("Path: expected %q, got %q", "/home/user/project", s.Path)
	}
	if s.Created.Unix() != 1700000000 {
		t.Errorf("Created: expected unix 1700000000, got %d", s.Created.Unix())
	}
	if s.Activity.Unix() != 1700000100 {
		t.Errorf("Activity: expected unix 1700000100, got %d", s.Activity.Unix())
	}
}

func TestParseSessions_attachedZeroMeansFalse(t *testing.T) {
	input := "detached|1700000000|0|1|1700000000|/tmp"
	sessions := ParseSessions(input)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Attached {
		t.Error("expected Attached=false when field is '0'")
	}
}

func TestParseSessions_multipleLines(t *testing.T) {
	input := "session-a|1700000000|1|2|1700000000|/a\nsession-b|1700000001|0|1|1700000001|/b"
	sessions := ParseSessions(input)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].Name != "session-a" {
		t.Errorf("first session name: expected %q, got %q", "session-a", sessions[0].Name)
	}
	if sessions[1].Name != "session-b" {
		t.Errorf("second session name: expected %q, got %q", "session-b", sessions[1].Name)
	}
}

func TestParseSessions_skipsEmptyLines(t *testing.T) {
	input := "\nsession-a|1700000000|1|1|1700000000|/a\n\nsession-b|1700000001|0|1|1700000001|/b\n"
	sessions := ParseSessions(input)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d (empty lines should be skipped)", len(sessions))
	}
}

func TestParseSessions_skipsLineWithFewerThanSixFields(t *testing.T) {
	input := "bad-line|only|four|fields"
	sessions := ParseSessions(input)
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for malformed line, got %d", len(sessions))
	}
}

func TestParseSessions_windowsInvalidBecomesZero(t *testing.T) {
	input := "s|1700000000|0|notanumber|1700000000|/path"
	sessions := ParseSessions(input)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Windows != 0 {
		t.Errorf("Windows: expected 0 for invalid string, got %d", sessions[0].Windows)
	}
}

func TestParseSessions_mixedValidAndInvalidLines(t *testing.T) {
	input := "good|1700000000|1|2|1700000000|/path\nbad-only-two\ngood2|1700000001|0|1|1700000001|/path2"
	sessions := ParseSessions(input)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 valid sessions, got %d", len(sessions))
	}
}

func TestParseUnixTimestamp_validTimestamp(t *testing.T) {
	ts := parseUnixTimestamp("1700000000")
	expected := time.Unix(1700000000, 0)
	if !ts.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, ts)
	}
}

func TestParseUnixTimestamp_invalidStringReturnsZeroTime(t *testing.T) {
	ts := parseUnixTimestamp("notanumber")
	if !ts.IsZero() {
		t.Errorf("expected zero time for invalid input, got %v", ts)
	}
}

func TestParseUnixTimestamp_emptyStringReturnsZeroTime(t *testing.T) {
	ts := parseUnixTimestamp("")
	if !ts.IsZero() {
		t.Errorf("expected zero time for empty input, got %v", ts)
	}
}

func TestParseUnixTimestamp_stripsWhitespace(t *testing.T) {
	ts := parseUnixTimestamp("  1700000000  ")
	expected := time.Unix(1700000000, 0)
	if !ts.Equal(expected) {
		t.Errorf("expected %v for input with surrounding spaces, got %v", expected, ts)
	}
}

func TestParseUnixTimestamp_zeroTimestamp(t *testing.T) {
	ts := parseUnixTimestamp("0")
	expected := time.Unix(0, 0)
	if !ts.Equal(expected) {
		t.Errorf("expected Unix epoch for '0', got %v", ts)
	}
}
