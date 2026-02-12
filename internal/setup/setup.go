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

	// Install mouse toggle script
	mouseTogglePath := filepath.Join(binDir, "claude-dashboard-mouse-toggle")
	if err := os.WriteFile(mouseTogglePath, mouseToggleScript, 0755); err != nil {
		return fmt.Errorf("failed to write mouse toggle script: %w", err)
	}

	// Install status bar script
	statusBarPath := filepath.Join(binDir, "claude-dashboard-status-bar")
	if err := os.WriteFile(statusBarPath, statusBarScript, 0755); err != nil {
		return fmt.Errorf("failed to write status bar script: %w", err)
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

	// Check if already configured
	if strings.Contains(existingConfig, "claude-dashboard-mouse-toggle") {
		return nil // Already configured
	}

	// Configuration to add
	config := `
# claude-dashboard: F12 key binding for mouse mode toggle
bind-key -n F12 run-shell "~/.local/bin/claude-dashboard-mouse-toggle"

# claude-dashboard: Status bar with version check and mouse status
set -g status-right-length 80
set -g status-right "#(~/.local/bin/claude-dashboard-status-bar) | [F12] Mouse:#{?mouse,ON,OFF} | %H:%M"
set -g status-interval 5

# claude-dashboard: Enable mouse mode by default
set -g mouse on
`

	// Append configuration
	f, err := os.OpenFile(tmuxConfPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open tmux config: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(config); err != nil {
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

// Setup performs the complete setup
func Setup(silent bool) error {
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

	if !silent {
		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("  🎉 Setup complete!")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
		fmt.Println("  Press F12 in tmux to toggle mouse mode")
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
	mouseTogglePath := filepath.Join(binDir, "claude-dashboard-mouse-toggle")
	statusBarPath := filepath.Join(binDir, "claude-dashboard-status-bar")

	// Check if both scripts exist
	if _, err := os.Stat(mouseTogglePath); err != nil {
		return false
	}
	if _, err := os.Stat(statusBarPath); err != nil {
		return false
	}

	return true
}
