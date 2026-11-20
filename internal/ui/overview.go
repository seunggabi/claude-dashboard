package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/seunggabi/claude-dashboard/internal/conversation"
	"github.com/seunggabi/claude-dashboard/internal/session"
	"github.com/seunggabi/claude-dashboard/internal/styles"
)

// RenderOverview renders one block per session: name and status, title,
// last prompt, and unfinished todos. Blocks scroll so the cursor's block
// stays inside height lines.
func RenderOverview(sessions []session.Session, summaries map[string]conversation.Summary, cursor, width, height int) string {
	if len(sessions) == 0 {
		return styles.Muted.Render("  No sessions found.") + "\n"
	}

	blocks := make([][]string, len(sessions))
	for i, s := range sessions {
		blocks[i] = overviewBlock(s, summaries[s.Name], i == cursor, width)
	}

	// Drop leading blocks until the cursor's block fits.
	start, used := 0, 0
	for i := 0; i <= cursor && i < len(blocks); i++ {
		used += len(blocks[i])
	}
	for start < cursor && used > height {
		used -= len(blocks[start])
		start++
	}

	var lines []string
	for _, block := range blocks[start:] {
		lines = append(lines, block...)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n") + "\n"
}

func overviewBlock(s session.Session, sum conversation.Summary, selected bool, width int) []string {
	textWidth := width - 8
	if textWidth < 10 {
		textWidth = 10
	}

	marker := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.SessionColor(s.Name))).Render("▌")
	var name string
	if selected {
		name = styles.Selected.Render(" " + truncate(s.Name, textWidth-14) + " ")
	} else {
		name = lipgloss.NewStyle().Bold(true).Render(truncate(s.Name, textWidth-12))
	}
	head := fmt.Sprintf("%s %s  %s", marker, name, s.StatusString())
	if s.Branch != "" {
		// Only what fits on the line, so the block never wraps.
		if room := min(30, width-lipgloss.Width(head)-2); room >= 5 {
			head += styles.Muted.Render("  " + truncate(s.Branch, room))
		}
	}
	lines := []string{head}

	title := sum.Title
	if title == "" {
		title = styles.Muted.Render("(no title yet)")
	} else {
		title, _, _ = strings.Cut(title, "\n")
		title = truncate(title, textWidth)
	}
	lines = append(lines, "    "+title)

	if sum.LastPrompt != "" {
		prompt, _, _ := strings.Cut(sum.LastPrompt, "\n")
		lines = append(lines, styles.Muted.Render("    > "+truncate(prompt, textWidth-2)))
	}

	if len(sum.Todos) > 0 {
		done := 0
		for _, t := range sum.Todos {
			if t.Status == "completed" {
				done++
			}
		}
		lines = append(lines, styles.Muted.Render(fmt.Sprintf("    todo %d/%d done", done, len(sum.Todos))))
		for _, t := range sum.Todos {
			t.Content, _, _ = strings.Cut(t.Content, "\n")
			switch t.Status {
			case "completed":
				continue
			case "in_progress":
				lines = append(lines, styles.Active.Render("    [>] "+truncate(t.Content, textWidth-4)))
			default:
				lines = append(lines, "    [ ] "+truncate(t.Content, textWidth-4))
			}
		}
	}

	return append(lines, "")
}
