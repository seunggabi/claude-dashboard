package setup

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed scripts/tmux-mouse-toggle.sh
var mouseToggleScript []byte

//go:embed scripts/tmux-status-bar.sh
var statusBarScript []byte

//go:embed scripts/tmux-save-history.sh
var saveHistoryScript []byte

// scriptInfo holds information about a helper script
type scriptInfo struct {
	name    string
	content []byte
}

var helperScripts = []scriptInfo{
	{"claude-dashboard-mouse-toggle", mouseToggleScript},
	{"claude-dashboard-status-bar", statusBarScript},
	{"claude-dashboard-save-history", saveHistoryScript},
}

// InstallScripts installs the helper scripts to ~/.local/bin
func InstallScripts() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	binDir := filepath.Join(homeDir, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Install all helper scripts
	for _, script := range helperScripts {
		scriptPath := filepath.Join(binDir, script.name)
		if err := os.WriteFile(scriptPath, script.content, 0755); err != nil {
			return fmt.Errorf("failed to write %s: %w", script.name, err)
		}
	}

	return nil
}

// SetupTmuxConfig adds the required tmux configuration
func SetupTmuxConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	tmuxConfPath := filepath.Join(homeDir, ".tmux.conf")

	// Read existing config
	var existingConfig string
	if data, err := os.ReadFile(tmuxConfPath); err == nil {
		existingConfig = string(data)
	}

	// Remove old/duplicate claude-dashboard configurations
	lines := strings.Split(existingConfig, "\n")
	var cleanedLines []string
	skipUntilBlank := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip old claude-dashboard comment blocks and their following lines
		if strings.HasPrefix(trimmed, "# Claude Dashboard -") ||
			strings.HasPrefix(trimmed, "# claude-dashboard:") {
			skipUntilBlank = true
			continue
		}

		// Skip lines that reference old scripts or duplicate bindings
		if strings.Contains(line, "claude-dashboard-version-check") ||
			strings.Contains(line, "claude-dashboard-mouse-toggle") ||
			strings.Contains(line, "claude-dashboard-status-bar") ||
			strings.Contains(line, "claude-dashboard-save-history") {
			continue
		}

		// Stop skipping when we hit a blank line after a comment block
		if skipUntilBlank && trimmed == "" {
			skipUntilBlank = false
			continue
		}

		if !skipUntilBlank {
			cleanedLines = append(cleanedLines, line)
		}
	}

	// Remove trailing empty lines
	for len(cleanedLines) > 0 && strings.TrimSpace(cleanedLines[len(cleanedLines)-1]) == "" {
		cleanedLines = cleanedLines[:len(cleanedLines)-1]
	}

	// Configuration to add
	config := `
# claude-dashboard: Increase scrollback buffer for full history capture
set -g history-limit 50000

# claude-dashboard: F12 key binding for mouse mode toggle
bind-key -n F12 run-shell "~/.local/bin/claude-dashboard-mouse-toggle"

# claude-dashboard: Ctrl+S key binding for saving pane history
bind-key -n C-s run-shell "~/.local/bin/claude-dashboard-save-history"

# claude-dashboard: Status bar with version check and mouse status
set -g status-right-length 80
set -g status-right "#(~/.local/bin/claude-dashboard-status-bar) | [F12] #[fg=#{?mouse,green,red}]Mouse:#{?mouse,ON,OFF}#[default] | %H:%M"
set -g status-interval 5

# claude-dashboard: Enable mouse mode by default
set -g mouse on

# claude-dashboard: Terminal overrides for better mouse support
set -g terminal-overrides 'xterm*:smcup@:rmcup@'
`

	// Write cleaned config with new configuration
	newConfig := strings.Join(cleanedLines, "\n") + config

	if err := os.WriteFile(tmuxConfPath, []byte(newConfig), 0644); err != nil {
		return fmt.Errorf("failed to write tmux config: %w", err)
	}

	return nil
}

// ReloadTmuxConfig reloads the tmux configuration
func ReloadTmuxConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	tmuxConfPath := filepath.Join(homeDir, ".tmux.conf")

	cmd := exec.Command("tmux", "source-file", tmuxConfPath)
	if err := cmd.Run(); err != nil {
		// Ignore error if tmux is not running
		return nil
	}

	return nil
}

// UpdateVersionCache updates the cached version information
func UpdateVersionCache(version string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache", "claude-dashboard")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Normalize version format (ensure it starts with 'v')
	if !strings.HasPrefix(version, "v") && version != "dev" {
		version = "v" + version
	}

	// Update current version cache
	currentVersionFile := filepath.Join(cacheDir, "current-version")
	if err := os.WriteFile(currentVersionFile, []byte(version), 0644); err != nil {
		return fmt.Errorf("failed to write current version cache: %w", err)
	}

	return nil
}

// Setup performs the complete setup
func Setup(silent bool, version string) error {
	if !silent {
		fmt.Println("🔧 Installing claude-dashboard helper scripts...")
	}

	if err := InstallScripts(); err != nil {
		return fmt.Errorf("failed to install scripts: %w", err)
	}

	if !silent {
		fmt.Println("✅ Helper scripts installed to ~/.local/bin/")
		fmt.Println()
		fmt.Println("🔧 Configuring tmux...")
	}

	if err := SetupTmuxConfig(); err != nil {
		return fmt.Errorf("failed to setup tmux config: %w", err)
	}

	if !silent {
		fmt.Println("✅ Tmux configuration added to ~/.tmux.conf")
		fmt.Println()
		fmt.Println("🔄 Reloading tmux configuration...")
	}

	if err := ReloadTmuxConfig(); err != nil {
		if !silent {
			fmt.Println("⚠️  Could not reload tmux (not running). Configuration will apply on next tmux start.")
		}
	} else {
		if !silent {
			fmt.Println("✅ Tmux configuration reloaded")
		}
	}

	// Update version cache
	if version != "" && version != "dev" {
		if !silent {
			fmt.Println()
			fmt.Println("🔄 Updating version cache...")
		}

		if err := UpdateVersionCache(version); err != nil {
			if !silent {
				fmt.Printf("⚠️  Warning: Could not update version cache: %v\n", err)
			}
		} else {
			if !silent {
				fmt.Println("✅ Version cache updated")
			}
		}
	}

	if !silent {
		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("  🎉 Setup complete!")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
		fmt.Println("  Press F12 in tmux to toggle mouse mode")
		fmt.Println("  Press Ctrl+S in tmux to save entire pane history to file")
		fmt.Println("  Check the status bar for version and mouse status")
		fmt.Println()
	}

	return nil
}

// CheckSetup checks if setup has been completed
func CheckSetup() bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	binDir := filepath.Join(homeDir, ".local", "bin")

	// Check if all helper scripts exist
	for _, script := range helperScripts {
		scriptPath := filepath.Join(binDir, script.name)
		if _, err := os.Stat(scriptPath); err != nil {
			return false
		}
	}

	return true
}
