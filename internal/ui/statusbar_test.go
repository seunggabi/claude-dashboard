package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestStatusBar_fitsOnOneLine(t *testing.T) {
	for _, width := range []int{60, 100, 163} {
		bar := StatusBar(width, 3, "dashboard", "api")
		if h := lipgloss.Height(bar); h != 1 {
			t.Errorf("width %d: status bar is %d lines high, want 1", width, h)
		}
		if w := lipgloss.Width(bar); w != width {
			t.Errorf("width %d: status bar is %d columns wide", width, w)
		}
	}
}
