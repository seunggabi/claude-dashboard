package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/seunggabi/claude-dashboard/internal/styles"
)

// StatusBar renders the bottom status bar.
func StatusBar(width int, sessionCount int, view string, filter string) string {
	left := styles.StatusKey.Render("Sessions: ") +
		styles.StatusVal.Render(fmt.Sprintf("%d", sessionCount))

	if filter != "" {
		left += "  " + styles.StatusKey.Render("Filter: ") +
			styles.StatusVal.Render(filter)
	}

	right := styles.StatusKey.Render("View: ") +
		styles.StatusVal.Render(view)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	bar := left + lipgloss.NewStyle().Width(gap).Render("") + right

	return styles.StatusBar.Width(width).Render(bar)
}

// HelpBar renders the key hints at the bottom.
func HelpBar(width int, context string) string {
	var hints string
	switch context {
	case "dashboard":
		hints = "↑/↓:nav  enter:attach  n:new  K:kill  l:logs  d:detail  /:filter  r:refresh  ?:help  q:quit"
	case "logs":
		hints = "↑/↓/j/k:scroll  pgup/pgdn:page  esc:back  q:quit"
	case "detail":
		hints = "esc:back  l:logs  K:kill  q:quit"
	case "create":
		hints = "tab:next  enter:create  esc:cancel"
	case "confirm":
		hints = "y:confirm  n:cancel"
	case "help":
		hints = "esc:close  q:quit"
	case "filter":
		hints = "enter:apply  esc:clear"
	default:
		hints = "?:help  q:quit"
	}

	return styles.Help.Width(width).Render(hints)
}
