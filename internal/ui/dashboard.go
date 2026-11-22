package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/seunggabi/claude-dashboard/internal/session"
	"github.com/seunggabi/claude-dashboard/internal/styles"
)

// minWidthFull is the minimum terminal width required to show PROGRESS and CLARITY columns.
const minWidthFull = 120

// DashboardColumns defines the table column widths.
// Width 0 means flexible (calculated dynamically).
var DashboardColumns = []struct {
	Title string
	Width int
}{
	{"#", 4},
	{"NAME", 0},      // flexible width
	{"PROJECT", 20},
	{"BRANCH", 20},
	{"STATUS", 12},
	{"UPTIME", 10},
	{"CPU", 8},
	{"MEM", 8},
	{"PROGRESS", 12}, // shown only when width >= minWidthFull
	{"CLARITY", 12},  // shown only when width >= minWidthFull
	{"PATH", 0},      // flexible width
}

// ProgressData carries progress information for one session.
type ProgressData struct {
	Done    int
	Total   int
	Percent int
	OK      bool
}

// fmtProgress formats a ProgressData for display.
// Returns "-" when OK is false or Total is zero.
func fmtProgress(pd ProgressData) string {
	if !pd.OK || pd.Total == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d %d%%", pd.Done, pd.Total, pd.Percent)
}

// clarityLevel returns the level string for a clarity score.
func clarityLevel(score int) string {
	switch {
	case score >= 70:
		return "clear"
	case score >= 40:
		return "vague"
	default:
		return "unclear"
	}
}

// fmtClarity formats a clarity score for display in a column of colWidth chars.
// Returns "-" for negative scores (issue task / no data).
// Shows "<score> <level>" when colWidth is large enough, otherwise just "<score>".
func fmtClarity(score, colWidth int) string {
	if score < 0 {
		return "-"
	}
	level := clarityLevel(score)
	full := fmt.Sprintf("%d %s", score, level)
	if colWidth >= len(full) {
		return full
	}
	return fmt.Sprintf("%d", score)
}

// RenderDashboard renders the session table with scroll support.
func RenderDashboard(sessions []session.Session, cursor int, width int, scrollOffset int, visibleRows int, progData map[string]ProgressData, clarityByName map[string]int) string {
	var b strings.Builder

	showExtra := width >= minWidthFull

	// Calculate flexible column widths.
	// PROGRESS (index 8) and CLARITY (index 9) are excluded in narrow mode.
	fixedWidth := 2 // left margin
	for i, col := range DashboardColumns {
		if col.Width > 0 {
			if (i == 8 || i == 9) && !showExtra {
				continue
			}
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
		DashboardColumns[8].Title,
		DashboardColumns[9].Title,
		DashboardColumns[10].Title,
		nameWidth, pathWidth, showExtra,
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

		var progress, clarity string
		if showExtra {
			pd := progData[s.Name]
			progress = fmtProgress(pd)
			score, ok := clarityByName[s.Name]
			if !ok {
				score = -1
			}
			clarity = fmtClarity(score, DashboardColumns[9].Width-2)
		}

		row := renderRow(
			fmt.Sprintf("%d", i+1),
			truncate(s.Name, nameWidth),
			truncate(s.Project, DashboardColumns[2].Width-2),
			truncate(s.Branch, DashboardColumns[3].Width-2),
			s.StatusString(),
			s.Uptime(),
			fmt.Sprintf("%.1f%%", s.CPU),
			fmt.Sprintf("%.1f%%", s.Memory),
			progress,
			clarity,
			truncatePath(s.Path, pathWidth),
			nameWidth, pathWidth, showExtra,
		)

		// The session's color marker takes the first margin column.
		marker := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.SessionColor(s.Name))).Render("▌")
		b.WriteString(marker)
		row = row[1:]

		if i == cursor {
			b.WriteString(styles.Selected.Width(width - 1).Render(row))
		} else if s.IsAsking() {
			b.WriteString(styles.Asking.Render(row))
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

func renderRow(idx, name, project, branch, status, uptime, cpu, mem, progress, clarity, path string, nameWidth, pathWidth int, showExtra bool) string {
	if showExtra {
		return fmt.Sprintf("  %-4s%-*s  %-20s%-20s%-12s%-10s%-8s%-8s%-12s%-12s%-*s",
			idx, nameWidth, name, project, branch, status, uptime, cpu, mem, progress, clarity, pathWidth, path)
	}
	return fmt.Sprintf("  %-4s%-*s  %-20s%-20s%-12s%-10s%-8s%-8s%-*s",
		idx, nameWidth, name, project, branch, status, uptime, cpu, mem, pathWidth, path)
}

func truncate(s string, maxLen int) string {
	if lipgloss.Width(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		runes := []rune(s)
		if maxLen > len(runes) {
			maxLen = len(runes)
		}
		return string(runes[:maxLen])
	}
	runes := []rune(s)
	if maxLen-3 > len(runes) {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
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
