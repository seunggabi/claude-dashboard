package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NoSetupEnv, when set to any non-empty value, stops claude-dashboard from
// offering to configure tmux on first run.
const NoSetupEnv = "CLAUDE_DASHBOARD_NO_SETUP"

// declinedMarker returns the file that records that the user declined the
// first-run setup offer, so they are not asked again on every launch.
func declinedMarker() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".cache", "claude-dashboard", "setup-declined"), nil
}

// IsDeclined reports whether the user previously declined the setup offer.
func IsDeclined() bool {
	path, err := declinedMarker()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// MarkDeclined remembers that the user declined the setup offer.
func MarkDeclined() error {
	path, err := declinedMarker()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}
	return os.WriteFile(path, []byte("declined\n"), 0644)
}

// ClearDeclined forgets a previous decline, e.g. after an explicit `setup`.
func ClearDeclined() error {
	path, err := declinedMarker()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Plan describes, in plain text, everything Setup would change. It is shown
// before the first-run prompt and by `setup --dry-run`.
func Plan() string {
	var b strings.Builder
	b.WriteString("Setup will make these changes:\n\n")
	b.WriteString("  1. Install helper scripts to ~/.local/bin/:\n")
	for _, s := range helperScripts {
		fmt.Fprintf(&b, "       - %s\n", s.name)
	}
	b.WriteString("\n  2. Back up ~/.tmux.conf (if it exists), then append to it:\n")
	b.WriteString("       - history-limit 50000\n")
	b.WriteString("       - F12 toggles mouse mode, Ctrl+S saves pane history\n")
	b.WriteString("         (these override any existing bindings for those keys)\n")
	b.WriteString("       - Ctrl+B g opens the dashboard in a popup\n")
	b.WriteString("       - status-right with session counts (replaces your status-right)\n")
	b.WriteString("       - mouse on, terminal-overrides for mouse support\n")
	b.WriteString("\n  The exact text appended to ~/.tmux.conf:\n")
	for _, line := range strings.Split(strings.TrimSpace(tmuxConfigBlock), "\n") {
		b.WriteString("    " + line + "\n")
	}
	return b.String()
}
