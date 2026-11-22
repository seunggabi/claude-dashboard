package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/seunggabi/claude-dashboard/internal/hooks"
)

// runSetupHooks registers (or removes) claude-dashboard's hooks in Claude Code's
// settings.json. It never changes anything without being asked: --dry-run only
// prints the plan, and a timestamped backup is written before any change.
func runSetupHooks(args []string) error {
	return setupHooks(args, os.Stdout)
}

func setupHooks(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("setup-hooks", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dryRun := fs.Bool("dry-run", false, "show what would change without writing")
	uninstall := fs.Bool("uninstall", false, "remove claude-dashboard's hooks")
	settings := fs.String("settings", "", "settings.json path (default ~/.claude/settings.json)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("usage: claude-dashboard setup-hooks [--dry-run] [--uninstall] [--settings PATH]: %w", err)
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	path := *settings
	if path == "" {
		path = hooks.DefaultPath()
	}

	if *dryRun {
		if *uninstall {
			return fmt.Errorf("--dry-run with --uninstall is not supported")
		}
		plan, err := hooks.Plan(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s\n(dry run: %s not modified)\n", plan, path)
		return nil
	}

	var backup string
	var err error
	if *uninstall {
		backup, err = hooks.Uninstall(path)
	} else {
		backup, err = hooks.Install(path)
	}
	if err != nil {
		return err
	}
	verb := "installed in"
	if *uninstall {
		verb = "removed from"
	}
	fmt.Fprintf(out, "claude-dashboard hooks %s %s\n", verb, path)
	if backup != "" {
		fmt.Fprintf(out, "backup: %s\n", backup)
	}
	return nil
}
