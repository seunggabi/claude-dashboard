package ui

import (
	"fmt"
	"strings"

	"github.com/seunggabi/claude-dashboard/internal/session"
	"github.com/seunggabi/claude-dashboard/internal/styles"
)

// RenderDetail renders the session detail view.
func RenderDetail(s *session.Session, width int) string {
	if s == nil {
		return styles.Error.Render("  No session selected")
	}

	var b strings.Builder

	title := styles.Title.Render(fmt.Sprintf(" Session Detail: %s ", s.Name))
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", width))
	b.WriteString("\n\n")

	rows := []struct {
		label string
		value string
	}{
		{"Name", s.Name},
		{"Project", s.Project},
		{"Status", s.StatusString()},
		{"Uptime", s.Uptime()},
		{"PID", s.PID},
		{"CPU", fmt.Sprintf("%.1f%%", s.CPU)},
		{"Memory", fmt.Sprintf("%.1f%%", s.Memory)},
		{"Path", s.Path},
		{"Branch", s.Branch},
		{"Attached", fmt.Sprintf("%v", s.Attached)},
		{"Started", s.StartedAt.Format("2006-01-02 15:04:05")},
	}

	for _, row := range rows {
		label := styles.DetailLabel.Render(row.label + ":")
		value := styles.DetailValue.Render(row.value)
		b.WriteString(fmt.Sprintf("  %s %s\n", label, value))
	}

	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", width))
	b.WriteString("\n")
	b.WriteString(styles.Help.Render("  Press 'l' for logs, 'K' to kill, 'esc' to go back"))
	b.WriteString("\n")

	return b.String()
}
