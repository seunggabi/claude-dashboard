package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/seunggabi/claude-dashboard/internal/app"
	"github.com/seunggabi/claude-dashboard/internal/config"
	"github.com/seunggabi/claude-dashboard/internal/session"
	"github.com/seunggabi/claude-dashboard/internal/setup"
)

var version = "dev"

func main() {
	// Claude Code hooks call this on every tool use: do nothing else first.
	if len(os.Args) > 1 && os.Args[1] == "hook" {
		if len(os.Args) > 2 {
			runHook(os.Args[2])
		}
		os.Exit(0)
	}

	app.Version = version
	session.SessionPrefix = config.Load().SessionPrefix
	app.DrainStdin()

	// Always update version cache on startup (important for Homebrew upgrades)
	// This is silent and fast, so it won't impact user experience
	if version != "" && version != "dev" {
		_ = setup.UpdateVersionCache(version)
	}

	// Auto-setup on first run (before any command)
	// Skip for --version, --help, and setup commands
	skipAutoSetup := len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v" ||
		os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "setup" ||
		os.Args[1] == "status" || // status output goes to the tmux status bar
		os.Args[1] == "queue" || os.Args[1] == "send" || os.Args[1] == "reply" || os.Args[1] == "dispatch" || os.Args[1] == "setup-hooks") // scripted: never prompt
	if !skipAutoSetup {
		runAutoSetup()
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("claude-dashboard %s\n", version)
			os.Exit(0)
		case "--help", "-h":
			printHelp()
			os.Exit(0)
		case "setup":
			if err := setup.Setup(false, version); err != nil {
				fmt.Fprintf(os.Stderr, "Setup failed: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "queue":
			finish(runQueueCommand(os.Args[2:]))
		case "send":
			finish(runSend(os.Args[2:]))
		case "reply":
			finish(runReply(os.Args[2:]))
		case "dispatch":
			finish(runDispatch(os.Args[2:]))
		case "setup-hooks":
			finish(runSetupHooks(os.Args[2:]))
		case "status":
			fmt.Print(app.StatusSummary())
			os.Exit(0)
		case "attach":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Usage: claude-dashboard attach <session-name>")
				os.Exit(1)
			}
			if err := app.ExecAttach(os.Args[2]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "new":
			name, path, claudeArgs := parseNewArgs(os.Args[2:])
			if path == "" {
				path, _ = os.Getwd()
			}
			if name == "" {
				homeDir, _ := os.UserHomeDir()
				name = defaultSessionName(path, homeDir)
			}
			sessionName := session.SessionPrefix + name

			err := app.CreateSession(name, path, claudeArgs)
			switch {
			case err == nil:
				fmt.Printf("Session '%s' created in %s\n", sessionName, path)
			case errors.Is(err, session.ErrSessionExists):
				fmt.Printf("Attaching to existing session '%s'...\n", sessionName)
			default:
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if err := app.ExecAttach(sessionName); err != nil {
				fmt.Fprintf(os.Stderr, "Error attaching: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runHook records a Claude Code hook state on the tmux session running this
// Claude, as "STATE:UNIXTIME". It never fails, so a hook never disturbs Claude.
func runHook(state string) {
	// Read the hook's JSON input so Claude never writes into a closed pipe.
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return
	}
	value := fmt.Sprintf("%s:%d", state, time.Now().Unix())
	_ = exec.Command("tmux", "set-option", "-q", "-t", pane, "@claude_state", value).Run()
	// Remember which pane runs Claude so text sent later is typed there and
	// not into whichever pane of the session has focus.
	_ = exec.Command("tmux", "set-option", "-q", "-t", pane, "@claude_pane", pane).Run()
}

// parseNewArgs parses the arguments that follow "new". A leading argument that
// is not a flag is the session name. --path and --args are consumed here;
// every other argument is forwarded to claude.
func parseNewArgs(args []string) (name, path, claudeArgs string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name = args[0]
		args = args[1:]
	}

	var forwarded []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--path":
			if i+1 < len(args) {
				path = args[i+1]
				i++
			}
		case "--args":
			if i+1 < len(args) {
				forwarded = append(forwarded, args[i+1])
				i++
			}
		default:
			forwarded = append(forwarded, args[i])
		}
	}
	return name, path, strings.Join(forwarded, " ")
}

// invalidNameChars matches characters that are not allowed in a session name.
var invalidNameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// defaultSessionName derives a session name from a directory: the path
// relative to home, with every invalid character replaced by "-".
// ~/project/my.app becomes project-my-app.
func defaultSessionName(path, homeDir string) string {
	rel := path
	if path == homeDir {
		rel = filepath.Base(path)
	} else if homeDir != "" && strings.HasPrefix(path, homeDir+"/") {
		rel = strings.TrimPrefix(path, homeDir+"/")
	}
	return invalidNameChars.ReplaceAllString(rel, "-")
}

// runAutoSetup runs first-time setup if not already configured.
func runAutoSetup() {
	if !setup.CheckSetup() {
		fmt.Println("📦 First time setup detected...")
		fmt.Println()
		if err := setup.Setup(false, version); err != nil {
			fmt.Fprintf(os.Stderr, "Auto-setup failed: %v\n", err)
			fmt.Println()
			fmt.Println("You can run 'claude-dashboard setup' manually later.")
			fmt.Println()
		}
	}
}

// finish reports err on stderr and exits; it exits 0 when err is nil.
func finish(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func printHelp() {
	fmt.Println(`claude-dashboard - k9s-style Claude Code Session Manager

Usage:
  claude-dashboard                                     Start the TUI dashboard
  claude-dashboard setup                               Install helper scripts and configure tmux
  claude-dashboard new [NAME] [options]                Create a new session (name defaults to path)
  claude-dashboard attach NAME                         Attach to a session directly
  claude-dashboard status                              Print waiting/active counts for the tmux status bar
  claude-dashboard queue add|list|rm|clear-done        Manage the task queue (add --issue 123, add --repo owner/name "text")
  claude-dashboard dispatch --sessions a,b | --all     Give queued tasks to idle sessions (--watch, --dry-run, --max-active N)
  claude-dashboard send SESSION TEXT                   Type one line into an idle session
  claude-dashboard reply SESSION TEXT | --asking TEXT  Answer a session (or all sessions) that asked a question
  claude-dashboard setup-hooks [--dry-run|--uninstall]  Register hooks in ~/.claude/settings.json (backup first)
  claude-dashboard hook STATE                          Record active/waiting/idle from a Claude Code hook
  claude-dashboard --version                           Show version
  claude-dashboard --help                              Show this help

New Session Options:
  --path <dir>         Working directory (default: current dir)
  --args <claude-args> Arguments to pass to claude (e.g. "--model opus")

Keybindings:
  enter   Attach to session
  n       New session
  K       Kill session
  ctrl+k  Kill all idle sessions
  l       View logs
  d       Session detail
  o       Overview (title, last prompt, todos)
  /       Filter
  r       Refresh
  ?       Help
  q       Quit

Requirements:
  - tmux must be installed

Config:
  ~/.claude-dashboard/config.yaml`)
}
