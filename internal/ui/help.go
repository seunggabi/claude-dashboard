package ui

import (
	"strings"

	"github.com/seunggabi/claude-dashboard/internal/styles"
)

// RenderHelp renders the help overlay.
func RenderHelp(width int) string {
	var b strings.Builder

	title := styles.Title.Render(" Help - Keybindings ")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", width))
	b.WriteString("\n\n")

	sections := []struct {
		title string
		keys  []struct{ key, desc string }
	}{
		{
			title: "Navigation",
			keys: []struct{ key, desc string }{
				{"↑/k", "Move up"},
				{"↓/j", "Move down"},
				{"enter", "Attach to session"},
				{"esc", "Go back / Cancel"},
			},
		},
		{
			title: "Actions",
			keys: []struct{ key, desc string }{
				{"n", "Create new session"},
				{"K", "Kill session (with confirm)"},
				{"l", "View session logs"},
				{"d", "View session detail"},
				{"r", "Refresh session list"},
			},
		},
		{
			title: "Logs Viewer",
			keys: []struct{ key, desc string }{
				{"↑/k", "Scroll up"},
				{"↓/j", "Scroll down"},
				{"pgup/pgdn", "Page up / down"},
				{"esc", "Back to dashboard"},
			},
		},
		{
			title: "Search & Other",
			keys: []struct{ key, desc string }{
				{"/", "Filter sessions"},
				{"?", "Show this help"},
				{"q", "Quit"},
				{"ctrl+c", "Force quit"},
			},
		},
	}

	for _, section := range sections {
		b.WriteString(styles.Header.Render("  " + section.title))
		b.WriteString("\n")
		for _, k := range section.keys {
			key := styles.StatusKey.Width(14).Render("  " + k.key)
			desc := styles.StatusVal.Render(k.desc)
			b.WriteString(key + desc + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(strings.Repeat("─", width))
	b.WriteString("\n")
	b.WriteString(styles.Help.Render("  Press esc or ? to close"))
	b.WriteString("\n")

	return b.String()
}
