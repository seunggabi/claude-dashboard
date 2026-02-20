package styles

import "github.com/charmbracelet/lipgloss"

// Colors
var (
	ColorPrimary   = lipgloss.Color("#7C3AED") // Purple
	ColorSecondary = lipgloss.Color("#06B6D4") // Cyan
	ColorSuccess   = lipgloss.Color("#10B981") // Green
	ColorWarning   = lipgloss.Color("#F59E0B") // Amber
	ColorDanger    = lipgloss.Color("#EF4444") // Red
	ColorMuted     = lipgloss.Color("#6B7280") // Gray
	ColorBg        = lipgloss.Color("#1F2937") // Dark bg
	ColorBgLight   = lipgloss.Color("#374151") // Light bg
	ColorText      = lipgloss.Color("#F9FAFB") // White
	ColorTextDim   = lipgloss.Color("#9CA3AF") // Dim text
)

// Styles
var (
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		PaddingLeft(1)

	StatusBar = lipgloss.NewStyle().
			Background(ColorBgLight).
			Foreground(ColorText).
			PaddingLeft(1).
			PaddingRight(1)

	StatusKey = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	StatusVal = lipgloss.NewStyle().
			Foreground(ColorTextDim)

	Active = lipgloss.NewStyle().
		Foreground(ColorSuccess).
		Bold(true)

	Waiting = lipgloss.NewStyle().
		Foreground(ColorWarning)

	Selected = lipgloss.NewStyle().
			Background(ColorPrimary).
			Foreground(ColorText).
			Bold(true)

	Help = lipgloss.NewStyle().
		Foreground(ColorTextDim)

	Error = lipgloss.NewStyle().
		Foreground(ColorDanger).
		Bold(true)

	Header = lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		Underline(true)

	Confirm = lipgloss.NewStyle().
		Foreground(ColorWarning).
		Bold(true)

	LogViewer = lipgloss.NewStyle().
			Padding(0, 1)

	DetailLabel = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true).
			Width(14)

	DetailValue = lipgloss.NewStyle().
			Foreground(ColorText)

	Muted = lipgloss.NewStyle().
		Foreground(ColorMuted)
)
