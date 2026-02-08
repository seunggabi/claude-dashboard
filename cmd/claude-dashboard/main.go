package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/seunggabi/claude-dashboard/internal/app"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("claude-dashboard %s\n", version)
			os.Exit(0)
		case "--help", "-h":
			printHelp()
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
			path, _ := os.Getwd()
			name := ""
			claudeArgs := ""
			attach := false

			// Parse args: first non-flag arg is name, rest are flags
			argStart := 2
			if len(os.Args) > 2 && !strings.HasPrefix(os.Args[2], "--") {
				name = os.Args[2]
				argStart = 3
			}

			for i := argStart; i < len(os.Args); i++ {
				switch os.Args[i] {
				case "--path":
					if i+1 < len(os.Args) {
						path = os.Args[i+1]
						i++
					}
				case "--args":
					if i+1 < len(os.Args) {
						claudeArgs = os.Args[i+1]
						i++
					}
				case "--attach":
					attach = true
				}
			}

			// Default name: path after home dir, e.g. ~/project/foo → project-foo
			if name == "" {
				homeDir, _ := os.UserHomeDir()
				rel := path
				if strings.HasPrefix(path, homeDir) {
					rel = strings.TrimPrefix(path, homeDir)
					rel = strings.TrimPrefix(rel, "/")
				}
				name = strings.ReplaceAll(rel, "/", "-")
				if name == "" {
					name = filepath.Base(path)
				}
			}

			if err := app.CreateSession(name, path, claudeArgs); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			sessionName := "cd-" + name
			fmt.Printf("Session '%s' created in %s\n", sessionName, path)
			if attach {
				if err := app.ExecAttach(sessionName); err != nil {
					fmt.Fprintf(os.Stderr, "Error attaching: %v\n", err)
					os.Exit(1)
				}
			}
			os.Exit(0)
		}
	}

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`claude-dashboard - k9s-style Claude Code Session Manager

Usage:
  claude-dashboard                                     Start the TUI dashboard
  claude-dashboard new [NAME] [options]                Create a new session (name defaults to path)
  claude-dashboard attach NAME                         Attach to a session directly
  claude-dashboard --version                           Show version
  claude-dashboard --help                              Show this help

New Session Options:
  --path <dir>         Working directory (default: current dir)
  --args <claude-args> Arguments to pass to claude (e.g. "--model opus")
  --attach             Attach to session after creation

Keybindings:
  enter   Attach to session
  n       New session
  K       Kill session
  l       View logs
  d       Session detail
  /       Filter
  r       Refresh
  ?       Help
  q       Quit

Requirements:
  - tmux must be installed

Config:
  ~/.claude-dashboard/config.yaml`)
}
