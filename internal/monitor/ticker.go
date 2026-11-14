package monitor

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TickMsg is sent on each refresh interval.
type TickMsg time.Time

// TickCmd returns a command that sends a TickMsg after the given interval.
func TickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}
