package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/seunggabi/claude-dashboard/internal/session"
)

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func managed(names ...string) []session.Session {
	sessions := make([]session.Session, len(names))
	for i, name := range names {
		sessions[i] = session.Session{Name: name, Managed: true, Status: session.StatusIdle}
	}
	return sessions
}

// The session named in the kill prompt must stay the target even if the list
// is refreshed, and the cursor now points at a different row, before the answer.
func TestKillTargetSurvivesRefresh(t *testing.T) {
	var m tea.Model = Model{sessions: managed("cd-a", "cd-b")}

	m, _ = m.Update(key("K"))
	m, _ = m.Update(SessionsMsg{Sessions: managed("cd-new", "cd-a", "cd-b")})

	got := m.(Model)
	if len(got.killTargets) != 1 || got.killTargets[0] != "cd-a" {
		t.Fatalf("kill targets = %v, want [cd-a]", got.killTargets)
	}
	if s, _ := got.selected(); s.Name != "cd-new" {
		t.Fatalf("test setup: cursor should now be on cd-new, got %q", s.Name)
	}
}

func TestKillIsRefusedForTerminalSessions(t *testing.T) {
	var m tea.Model = Model{sessions: []session.Session{{Name: "terminal/pts/1"}}}

	m, _ = m.Update(key("K"))

	got := m.(Model)
	if got.killTargets != nil || got.err == nil {
		t.Fatalf("expected an error and no kill prompt, got targets=%v err=%v", got.killTargets, got.err)
	}
}

func TestCursorIsClampedToFilteredList(t *testing.T) {
	m := Model{sessions: managed("cd-api", "cd-web", "cd-infra"), cursor: 2, scrollOffset: 2}

	m.filterQuery = "api"
	m.clampCursor()

	if m.cursor != 0 || m.scrollOffset != 0 {
		t.Fatalf("cursor=%d scrollOffset=%d, want 0 and 0", m.cursor, m.scrollOffset)
	}
	if s, ok := m.selected(); !ok || s.Name != "cd-api" {
		t.Fatalf("selected = %q (ok=%v), want cd-api", s.Name, ok)
	}

	m.filterQuery = "no-match"
	m.clampCursor()
	if _, ok := m.selected(); ok || m.cursor != 0 {
		t.Fatalf("empty list should select nothing with cursor 0, got cursor=%d", m.cursor)
	}
}
