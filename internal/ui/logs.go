package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/seunggabi/claude-dashboard/internal/styles"
)

// LogView holds the log viewer state.
type LogView struct {
	Viewport    viewport.Model
	SessionName string
	Ready       bool
}

// NewLogView creates a new log viewer.
func NewLogView(sessionName string, width, height int) LogView {
	vp := viewport.New(width, height-4)
	vp.Style = styles.LogViewer

	return LogView{
		Viewport:    vp,
		SessionName: sessionName,
	}
}

// SetContent updates the log content.
func (l *LogView) SetContent(content string) {
	l.Viewport.SetContent(content)
	l.Viewport.GotoBottom()
	l.Ready = true
}

// SetSize updates the viewport dimensions.
func (l *LogView) SetSize(width, height int) {
	l.Viewport.Width = width
	l.Viewport.Height = height - 4
}

// RenderLogView renders the log viewer.
func RenderLogView(lv LogView, width int) string {
	var b strings.Builder

	title := styles.Title.Render(fmt.Sprintf(" Logs: %s ", lv.SessionName))
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", width))
	b.WriteString("\n")

	if !lv.Ready {
		b.WriteString("\n  Loading logs...")
	} else {
		b.WriteString(lv.Viewport.View())
	}

	b.WriteString("\n")

	scrollInfo := styles.Muted.Render(
		fmt.Sprintf(" %3.f%% ", lv.Viewport.ScrollPercent()*100),
	)
	bar := lipgloss.PlaceHorizontal(width, lipgloss.Right, scrollInfo)
	b.WriteString(bar)

	return b.String()
}
