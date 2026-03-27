# claude-dashboard

**k9s-style TUI for managing Claude Code sessions with real-time monitoring, conversation history, and process-based session detection.**

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

### Homebrew

```bash
brew install seunggabi/tap/claude-dashboard
# Setup is automatic on first run, or run manually:
claude-dashboard setup
```

### Manual Installation

```bash
curl -fsSL https://raw.githubusercontent.com/seunggabi/claude-dashboard/main/install.sh | bash
# Setup is automatic on first run, or run manually:
claude-dashboard setup
```

**Setup includes:**
- ✅ Installs helper scripts to `~/.local/bin/`
- ✅ Configures `~/.tmux.conf` for F12 mouse toggle and Ctrl+S history save
- ✅ Adds status bar with version info
- ✅ Enables mouse mode by default

This deploys the binary to `~/.local/bin`. For a custom binary name:

```bash
~/.local/bin/claude-dashboard
```

```bash
curl -fsSL https://raw.githubusercontent.com/seunggabi/claude-dashboard/main/install.sh | bash -s -- --name <your-binary-name>
```

### Install via Go

```bash
go install github.com/seunggabi/claude-dashboard/cmd/claude-dashboard@latest
```

### Build from Source

```bash
git clone https://github.com/seunggabi/claude-dashboard.git
cd claude-dashboard
make install
```

First run:

```bash
claude-dashboard   # Launch the TUI dashboard
```


### 💡 Pro Tip: Super Fast Session Creation

Type just **`cdn`** instead of `claude-dashboard new` - save time with every session!

```bash
# One-line setup (add to your ~/.zshrc or ~/.bashrc for permanent use)
source <(curl -fsSL https://raw.githubusercontent.com/seunggabi/claude-dashboard/main/alias.sh)
```

**Example:**
```bash
# Before: claude-dashboard new my-project --path ~/code/foo
# After:  cdn my-project --path ~/code/foo  ⚡️

# Resume or continue conversations instantly
cdn -r                                 # Resume (interactive picker)
cdn -c                                 # Continue most recent conversation
cdn my-project -r                      # Resume with session name
```

### 🔄 Upgrade

Keep claude-dashboard up to date with the latest features:

```bash
# Homebrew
brew update
brew upgrade claude-dashboard

# Manual installation
curl -fsSL https://raw.githubusercontent.com/seunggabi/claude-dashboard/main/install.sh | bash

# Go
go install github.com/seunggabi/claude-dashboard/cmd/claude-dashboard@latest
```

**After upgrading, run setup to apply new configurations:**
```bash
claude-dashboard setup
```

## Why claude-dashboard

### The Problem

Running multiple Claude Code sessions across different projects quickly becomes unmanageable:

- **Lost sessions** - Terminal closed? Session gone. What was Claude doing?
- **No overview** - Which session is active? How long has it been running? What are resource usage patterns?
- **Context switching** - Constantly hunting through terminal tabs, tmux windows, and scattered `.jsonl` logs.
- **Session discovery** - Sessions hidden in tmux, terminal tabs, or started by other tools.

### The Solution

claude-dashboard gives you a **single pane of glass** for all your Claude Code sessions:

- **Unified session detection** - Finds Claude sessions in tmux, terminal tabs, and anywhere in the process tree
- **Conversation history** - View Claude's past interactions directly from the dashboard
- **Real-time monitoring** - CPU, memory, status, and uptime at a glance
- **Session persistence** - Sessions keep running in tmux; detach anytime and come back
- **Single binary** - One `brew install` and you're done

Sessions run inside tmux - close your terminal, shut your laptop, and come back anytime. [k9s](https://k9scli.io/)-style vim navigation with real-time CPU/memory monitoring.

## Keybindings

### Dashboard

| Key       | Action                                    |
|-----------|-------------------------------------------|
| `↑` / `k` | Move cursor up                            |
| `↓` / `j` | Move cursor down                          |
| `enter`   | Attach to session                         |
| `n`       | Create new session                        |
| `K`       | Kill session (with confirmation)          |
| `Ctrl+K`  | Kill all idle sessions (with confirmation)|
| `l`       | View session logs                         |
| `Ctrl+S`  | Save entire pane history to file (in attached session) |
| `d`       | Session detail view                       |
| `/`       | Filter / search sessions                  |
| `r`       | Manual refresh                            |
| `?`       | Help overlay                              |
| `esc`     | Go back / cancel                          |
| `q`       | Quit                                      |

### Logs Viewer

| Key             | Action            |
|-----------------|-------------------|
| `↑` / `k`       | Scroll up         |
| `↓` / `j`       | Scroll down       |
| `PgUp` / `PgDn` | Page up / down (macOS: `Fn+↑` / `Fn+↓`) |
| `esc`           | Back to dashboard |
| `q`             | Quit              |

## Features

- **Session Dashboard** - All Claude sessions in one table with real-time status, CPU/memory, and uptime (auto-refreshes every 2s). Detects managed sessions, tmux sessions, process tree, and terminal tabs.
- **Conversation Log Viewer** (`l`) - Browse conversation history from `~/.claude/projects/` `.jsonl` files. Read-only, no attachment needed.
- **Attach / Detach** (`enter` / `Ctrl+B d`) - Attach to tmux sessions; terminal sessions are read-only.

### Tips

| Action | How |
|--------|-----|
| **Copy text (macOS)** | Hold `Option (⌥)` + drag, then `Cmd+C` |
| **Copy text (Linux)** | Hold `Shift` + drag, then `Ctrl+Shift+C` |
| **Scroll history** | `Ctrl+B [` to enter copy mode, `q` to exit |
| **Toggle mouse** | `F12` (ON: scroll with mouse, OFF: easy text select) |
| **Save pane history** | `Ctrl+S` in attached session (saves to `~/Desktop/`) |

### Create Session

**TUI**: Press `n` to create interactively. **CLI**:

```bash
claude-dashboard new                   # Auto-name from current directory
claude-dashboard new my-project        # Explicit name
claude-dashboard new --path ~/project  # Specify directory
claude-dashboard new --args "--model opus"
```

#### Claude CLI Pass-through Options

Flags not recognized by claude-dashboard (`--path`, `--args`) are forwarded to `claude`:

| Flag | Description |
|------|-------------|
| `-r`, `--resume` | Resume a conversation by session ID, or open interactive picker |
| `-c`, `--continue` | Continue the most recent conversation in the current directory |
| `--model <model>` | Specify model (e.g., `opus`, `sonnet`) |

```bash
claude-dashboard new my-project -c
claude-dashboard new my-project --path ~/code/foo -r
```

If a session with the same name already exists, it automatically attaches instead.

## Status Detection

| Status | Indicator | Description |
|--------|-----------|-------------|
| `● active` | Green | Output is streaming |
| `○ idle` | Gray | Prompt visible, no activity |
| `◎ waiting` | Amber | Input prompt or Y/n question |
| `⊘ terminal` | Blue | Claude in terminal tab (read-only) |

## Configuration

`~/.claude-dashboard/config.yaml`:

```yaml
refresh_interval: 2s       # Auto-refresh interval
session_prefix: "cd-"      # Prefix for managed sessions
default_dir: ""            # Default project directory for new sessions
log_history: 1000          # Number of log lines to capture
```

## Requirements

- **tmux** (session backend)
- **Go 1.25+** (only for building from source)

### Install tmux

```bash
# macOS
brew install tmux

# Ubuntu/Debian
sudo apt install tmux
```

## Usage

```bash
claude-dashboard                       # Launch TUI dashboard
claude-dashboard new [name]            # Create session (auto-name if omitted)
claude-dashboard attach <session>      # Attach directly (skip TUI)
claude-dashboard setup                 # Install helper scripts & configure tmux
claude-dashboard --version             # Show version
claude-dashboard --help                # Show help
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
│   │   ├── detector.go               # Discover sessions from tmux/terminal/processes
│   │   └── manager.go                # CRUD operations
│   ├── tmux/                         # tmux integration
│   │   ├── client.go                 # Command wrapper
│   │   └── parser.go                 # Output parser
│   ├── conversation/                 # Conversation history
│   │   └── reader.go                 # Parse .jsonl files from ~/.claude/projects/
│   ├── ui/                           # View components
│   │   ├── dashboard.go              # Session table
│   │   ├── logs.go                   # Log viewer (viewport)
│   │   ├── detail.go                 # Detail view
│   │   ├── create.go                 # New session form
│   │   ├── help.go                   # Help overlay
│   │   └── statusbar.go             # Status bar
│   ├── monitor/                      # Resource monitoring
│   │   ├── process.go                # CPU/memory via ps, process tree BFS
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
