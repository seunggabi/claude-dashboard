package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/seunggabi/claude-dashboard/internal/session"
	"github.com/seunggabi/claude-dashboard/internal/styles"
)

// DashboardColumns defines the table column widths.
// Width 0 means flexible (calculated dynamically).
var DashboardColumns = []struct {
	Title string
	Width int
}{
	{"#", 4},
	{"NAME", 0},    // flexible width
	{"PROJECT", 35},
	{"STATUS", 12},
	{"UPTIME", 10},
	{"CPU", 8},
	{"MEM", 8},
	{"PATH", 0}, // flexible width
}

// RenderDashboard renders the session table with scroll support.
func RenderDashboard(sessions []session.Session, cursor int, width int, scrollOffset int, visibleRows int) string {
	var b strings.Builder

	// Calculate flexible column widths
	fixedWidth := 2 // left margin
	for _, col := range DashboardColumns {
		if col.Width > 0 {
			fixedWidth += col.Width + 2
		}
	}
	flexWidth := width - fixedWidth
	if flexWidth < 30 {
		flexWidth = 30
	}
	nameWidth := flexWidth / 3
	pathWidth := flexWidth - nameWidth

	// Header
	header := renderRow(
		DashboardColumns[0].Title,
		DashboardColumns[1].Title,
		DashboardColumns[2].Title,
		DashboardColumns[3].Title,
		DashboardColumns[4].Title,
		DashboardColumns[5].Title,
		DashboardColumns[6].Title,
		DashboardColumns[7].Title,
		nameWidth, pathWidth,
	)
	b.WriteString(styles.Header.Render(header))
	b.WriteString("\n")

	if len(sessions) == 0 {
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render("  No sessions found. Press 'n' to create a new session."))
		b.WriteString("\n")
		return b.String()
	}

	// Determine visible range
	end := scrollOffset + visibleRows
	if end > len(sessions) {
		end = len(sessions)
	}

	// Scroll indicator (top)
	if scrollOffset > 0 {
		indicator := styles.Muted.Render(fmt.Sprintf("  ▲ %d more above", scrollOffset))
		b.WriteString(indicator)
		b.WriteString("\n")
	}

	// Rows (only visible range)
	for i := scrollOffset; i < end; i++ {
		s := sessions[i]
		row := renderRow(
			fmt.Sprintf("%d", i+1),
			truncate(s.Name, nameWidth),
			truncate(s.Project, DashboardColumns[2].Width),
			s.StatusString(),
			s.Uptime(),
			fmt.Sprintf("%.1f%%", s.CPU),
			fmt.Sprintf("%.1f%%", s.Memory),
			truncatePath(s.Path, pathWidth),
			nameWidth, pathWidth,
		)

		if i == cursor {
			b.WriteString(styles.Selected.Width(width).Render(row))
		} else {
			switch s.Status {
			case session.StatusActive:
				b.WriteString(styles.Active.Render(row))
			case session.StatusWaiting:
				b.WriteString(styles.Waiting.Render(row))
			default:
				b.WriteString(row)
			}
		}
		b.WriteString("\n")
	}

	// Scroll indicator (bottom)
	if end < len(sessions) {
		indicator := styles.Muted.Render(fmt.Sprintf("  ▼ %d more below", len(sessions)-end))
		b.WriteString(indicator)
		b.WriteString("\n")
	}

	return b.String()
}

func renderRow(idx, name, project, status, uptime, cpu, mem, path string, nameWidth, pathWidth int) string {
	return fmt.Sprintf("  %-4s%-*s  %-35s%-12s%-10s%-8s%-8s%-*s",
		idx, nameWidth, name, project, status, uptime, cpu, mem, pathWidth, path)
}

func truncate(s string, maxLen int) string {
	if lipgloss.Width(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func truncatePath(s string, maxLen int) string {
	if maxLen <= 0 || lipgloss.Width(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..."
	}
	if len(s) > maxLen {
		return "..." + s[len(s)-(maxLen-3):]
	}
	return s
}
