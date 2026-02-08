# claude-dashboard

**k9s-style TUI for managing Claude Code sessions via tmux.**

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](https://opensource.org/licenses/MIT)
[![GitHub Stars](https://img.shields.io/github/stars/seunggabi/claude-dashboard?style=social)](https://github.com/seunggabi/claude-dashboard/stargazers)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev)
[![Latest Release](https://img.shields.io/github/v/release/seunggabi/claude-dashboard?color=blue)](https://github.com/seunggabi/claude-dashboard/releases)
[![tmux](https://img.shields.io/badge/tmux-required-1BB91F)](https://github.com/tmux/tmux)

---

## Demo

<!-- Record with: vhs demo/demo.tape -->

<p align="center"><img src="demo/demo.gif" alt="claude-dashboard demo" width="720"></p>

## Quick Start

```bash
# Install via Go
go install github.com/seunggabi/claude-dashboard/cmd/claude-dashboard@latest

# Add Go bin to PATH (if not already configured)
export PATH="$HOME/go/bin:$PATH"

# To make it permanent, add to your shell profile:
# echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc   # zsh
# echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.bashrc  # bash
```

Or build from source:

```bash
git clone https://github.com/seunggabi/claude-dashboard.git
cd claude-dashboard
make install
```

First run:

```bash
claude-dashboard   # Launch the TUI dashboard
```

## Why claude-dashboard

### The Problem

Running multiple Claude Code sessions across different projects quickly becomes unmanageable:

- **Lost sessions** - Terminal closed? Session gone.
- **No overview** - Which session is doing what? No way to tell at a glance.
- **Context switching** - Constantly hunting through terminal tabs and tmux windows.

### The Solution

claude-dashboard gives you a **single pane of glass** for all your Claude Code sessions, powered by tmux for session persistence.

### Session Persistence

Every Claude Code session runs inside tmux. Close your terminal, shut your laptop - sessions keep running. Come back anytime and re-attach exactly where you left off.

### Real-time Monitoring

| Column | Description |
|--------|-------------|
| Name | Session identifier |
| Project | Project directory name |
| Status | `● active` / `○ idle` / `◎ waiting` |
| Uptime | Time since session creation |
| CPU | CPU usage (process tree) |
| Memory | Memory usage (process tree) |
| Path | Working directory |

### k9s-style Keybindings

If you've used [k9s](https://k9scli.io/), you'll feel right at home. Vim-style navigation, single-key actions, instant feedback.

## Keybindings

| Key | Action |
|-----|--------|
| `enter` | Attach to session |
| `n` | Create new session |
| `K` | Kill session (with confirmation) |
| `l` | View session logs |
| `d` | Session detail view |
| `/` | Filter / search sessions |
| `r` | Manual refresh |
| `?` | Help overlay |
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `esc` | Go back / cancel |
| `q` | Quit |

## Features

### Session Dashboard

View all Claude Code sessions in a table with real-time status, resource usage, and uptime. Auto-refreshes every 2 seconds.

### Attach / Detach

Press `enter` to attach to any session. Use `Ctrl+B d` (tmux detach) to return to the dashboard. Sessions continue running in the background.

### Log Viewer

Press `l` to view captured terminal output with a scrollable viewport. Useful for checking what Claude is doing without attaching.

### New Session

Press `n` to create a new session. Enter a name and project directory - claude-dashboard creates a tmux session running `claude` in that directory.

### Kill Session

Press `K` to terminate a session. Always shows a confirmation prompt before killing (safety first).

### Filter / Search

Press `/` to filter sessions by name, project, status, or path. Press `esc` to clear the filter.

### Session Detail

Press `d` for a detailed view showing PID, CPU, memory, path, start time, and attached status.

## Session Naming

| Type | Pattern | Example |
|------|---------|---------|
| Managed sessions | `cd-<name>` prefix | `cd-my-project` |
| Detected sessions | Contains "claude" | `claude-api-work` |

Sessions created through claude-dashboard use the `cd-` prefix. Existing tmux sessions with "claude" in the name are also detected automatically.

## Status Detection

Session status is determined by analyzing tmux pane content:

| Status | Indicator | Detection |
|--------|-----------|-----------|
| `● active` | Green | Output is streaming |
| `○ idle` | Gray | Prompt visible, no activity |
| `◎ waiting` | Amber | Input prompt or Y/n question |
| `? unknown` | - | Unable to determine |

## Configuration

`~/.claude-dashboard/config.yaml`:

```yaml
refresh_interval: 2s       # Auto-refresh interval
session_prefix: "cd-"      # Prefix for managed sessions
default_dir: ""            # Default project directory for new sessions
log_history: 1000          # Number of log lines to capture
theme: dark                # UI theme
```

## Requirements

- **Go 1.25+** (for building)
- **tmux** (session backend)

```bash
# macOS
brew install tmux

# Ubuntu/Debian
sudo apt install tmux
```

## Usage

```bash
# Launch TUI dashboard
claude-dashboard

# Attach to a session directly (skip TUI)
claude-dashboard attach cd-my-project

# Show version
claude-dashboard --version

# Show help
claude-dashboard --help
```

## Project Structure

```
claude-dashboard/
├── cmd/claude-dashboard/main.go      # CLI entry point
├── internal/
│   ├── app/                          # Bubble Tea application
│   │   ├── app.go                    # Main model, Update, View
│   │   └── keys.go                   # Keybinding definitions
│   ├── session/                      # Session management
│   │   ├── session.go                # Session data model
│   │   ├── detector.go               # Discover sessions from tmux
│   │   └── manager.go                # CRUD operations
│   ├── tmux/                         # tmux integration
│   │   ├── client.go                 # Command wrapper
│   │   └── parser.go                 # Output parser
│   ├── ui/                           # View components
│   │   ├── dashboard.go              # Session table
│   │   ├── logs.go                   # Log viewer (viewport)
│   │   ├── detail.go                 # Detail view
│   │   ├── create.go                 # New session form
│   │   ├── help.go                   # Help overlay
│   │   └── statusbar.go             # Status bar
│   ├── monitor/                      # Resource monitoring
│   │   ├── process.go                # CPU/memory via ps
│   │   └── ticker.go                 # Periodic refresh
│   ├── config/config.go              # YAML configuration
│   └── styles/styles.go              # Lipgloss styles
├── LICENSE                           # MIT
├── Makefile                          # build, install, clean
└── .goreleaser.yml                   # Release automation
```

## Tech Stack

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Elm architecture TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components (table, viewport, textinput)
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Style definitions
- [tmux](https://github.com/tmux/tmux) - Terminal multiplexer for session persistence

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=seunggabi/claude-dashboard&type=Date)](https://star-history.com/#seunggabi/claude-dashboard&Date)

## License

[MIT](LICENSE)

<!-- GitHub Topics: claude, claude-code, tui, tmux, session-manager, terminal, go, bubbletea, k9s, cli -->
