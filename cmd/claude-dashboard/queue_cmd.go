package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/seunggabi/claude-dashboard/internal/clarity"
	"github.com/seunggabi/claude-dashboard/internal/config"
	"github.com/seunggabi/claude-dashboard/internal/dispatch"
	"github.com/seunggabi/claude-dashboard/internal/issue"
	"github.com/seunggabi/claude-dashboard/internal/queue"
	"github.com/seunggabi/claude-dashboard/internal/session"
	"github.com/seunggabi/claude-dashboard/internal/tmux"
)

// minWatchInterval keeps a dispatcher from polling tmux in a tight loop.
const minWatchInterval = 2 * time.Second

// newFlagSet returns a FlagSet that reports errors instead of exiting.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// stdinIsTerminal reports whether a person could answer a prompt on stdin.
// /dev/null is a character device too, but a script run with </dev/null has
// nobody to ask, so it does not count.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	null, err := os.Stat(os.DevNull)
	return err != nil || !os.SameFile(info, null)
}

// confirm asks a yes/no question; anything but y/yes is a no.
func confirm(prompt string) bool {
	fmt.Print(prompt + " [y/N] ")
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(answer))
	return a == "y" || a == "yes"
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// guidelinesPath is the file whose text is appended to every queued task.
func guidelinesPath() string {
	return filepath.Join(config.ConfigDir(), "guidelines.txt")
}

// loadGuidelines reads the shared guidelines as one line ("" when the file
// does not exist). Line breaks and tabs become single spaces because a task is
// typed into a session as one line.
func loadGuidelines() (string, error) {
	data, err := os.ReadFile(guidelinesPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", guidelinesPath(), err)
	}
	return strings.Join(strings.Fields(string(data)), " "), nil
}

// ---------------------------------------------------------------- queue

// runQueueCommand handles `claude-dashboard queue add|list|rm|clear-done`.
func runQueueCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: claude-dashboard queue add|list|rm|clear-done")
	}
	q := queue.Open(queue.DefaultPath())
	switch args[0] {
	case "add":
		return queueAdd(q, args[1:])
	case "list", "ls":
		return queueList(q)
	case "rm":
		if len(args) != 2 {
			return errors.New("usage: claude-dashboard queue rm <id>")
		}
		var id int
		if _, err := fmt.Sscanf(args[1], "%d", &id); err != nil || id < 1 {
			return fmt.Errorf("invalid task id %q", args[1])
		}
		if err := q.Remove(id); err != nil {
			return err
		}
		fmt.Printf("Removed task %d\n", id)
		return nil
	case "clear-done":
		n, err := q.ClearDispatched()
		if err != nil {
			return err
		}
		fmt.Printf("Removed %d dispatched task(s)\n", n)
		return nil
	default:
		return fmt.Errorf("unknown queue command %q", args[0])
	}
}

func queueAdd(q *queue.Queue, args []string) error {
	fs := newFlagSet("queue add")
	issueRef := fs.String("issue", "", "GitHub issue: 123, #123, owner/name#123 or a URL")
	repo := fs.String("repo", "", "only sessions working in a clone of owner/name may take the task")
	noGuidelines := fs.Bool("no-guidelines", false, "do not append "+guidelinesPath())
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("queue add: %w", err)
	}

	guidelines := ""
	if !*noGuidelines {
		g, err := loadGuidelines()
		if err != nil {
			return err
		}
		guidelines = g
	}

	var (
		text, brief, taskRepo, label string
		scored                       *clarity.Result
	)
	if *issueRef != "" {
		if fs.NArg() > 0 {
			return errors.New("--issue does not take free text")
		}
		refRepo, number, err := issue.ParseRef(*issueRef)
		if err != nil {
			return err
		}
		if refRepo != "" && *repo != "" && refRepo != *repo {
			return fmt.Errorf("--repo %s conflicts with the repository in %q", *repo, *issueRef)
		}
		taskRepo = *repo
		if taskRepo == "" {
			taskRepo = refRepo
		}
		if taskRepo == "" {
			if cwd, err := os.Getwd(); err == nil {
				taskRepo = issue.OriginRepo(cwd)
			}
		}
		if taskRepo == "" {
			return errors.New("cannot tell which repository the issue belongs to: pass --repo owner/name")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		is, err := issue.Fetch(ctx, issue.DefaultRunner, taskRepo, number)
		if err != nil {
			return err
		}
		if text, err = is.Prompt(guidelines); err != nil {
			return err
		}
		label = fmt.Sprintf("#%d %s", is.Number, is.Title)
	} else {
		raw := strings.TrimSpace(strings.Join(fs.Args(), " "))
		if raw == "" {
			return errors.New("usage: claude-dashboard queue add [--repo owner/name] [--no-guidelines] <task text> | --issue <ref>")
		}
		taskRepo = *repo
		// Score what the user wrote, not the shared guidelines appended to it.
		r := clarity.Score(raw)
		scored = &r
		text, brief = raw, raw
		if guidelines != "" {
			text += " " + guidelines
		}
	}

	task, err := q.AddWithBrief(text, brief, taskRepo, label)
	if err != nil {
		return err
	}
	fmt.Printf("Queued task %d", task.ID)
	if task.Repo != "" {
		fmt.Printf(" for %s", task.Repo)
	}
	fmt.Println()
	if scored != nil {
		fmt.Printf("Clarity %d (%s) - a rough heuristic, not a measurement\n", scored.Score, scored.Level)
		if scored.Level != "clear" {
			for _, reason := range scored.Reasons {
				fmt.Println("  " + reason)
			}
			fmt.Println("Consider making the task more specific before it is dispatched.")
		}
	}
	return nil
}

func queueList(q *queue.Queue) error {
	tasks, err := q.List()
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		fmt.Println("The queue is empty.")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tSESSION\tREPO\tCLARITY\tTASK")
	for _, t := range tasks {
		// Score what the user wrote, never the shared guidelines appended to it.
		own := t.Brief
		if own == "" {
			own = t.Text
		}
		score := "n/a" // issue tasks use a fixed sentence; the issue text is not scored
		if t.Ref == "" {
			r := clarity.Score(own)
			score = fmt.Sprintf("%d %s", r.Score, r.Level)
		}
		what := t.Ref
		if what == "" {
			what = truncate(own, 60)
		}
		repo := t.Repo
		if repo == "" {
			repo = "-"
		}
		sess := t.Session
		if sess == "" {
			sess = "-"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", t.ID, t.Status, sess, repo, score, what)
	}
	return tw.Flush()
}

// ----------------------------------------------------- send and reply

// detect lists the Claude sessions using a fresh tmux client.
func detect(ctx context.Context) (*tmux.Client, []session.Session, error) {
	client, err := tmux.NewClient()
	if err != nil {
		return nil, nil, err
	}
	sessions, err := session.NewDetector(client).Detect(ctx)
	return client, sessions, err
}

// runSend handles `claude-dashboard send [--queue-behind-work] <session> <text>`.
func runSend(args []string) error {
	fs := newFlagSet("send")
	behind := fs.Bool("queue-behind-work", false, "also send to a session that is busy (Claude Code handles it after the current work)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	if fs.NArg() < 2 {
		return errors.New("usage: claude-dashboard send [--queue-behind-work] <session> <text>")
	}
	mode := session.ModeIdleOnly
	if *behind {
		mode = session.ModeQueueBehindWork
	}
	return sendOne(fs.Arg(0), strings.Join(fs.Args()[1:], " "), mode)
}

func sendOne(name, text string, mode session.SendMode) error {
	if err := tmux.ValidateSendText(text); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, sessions, err := detect(ctx)
	if err != nil {
		return err
	}
	for i := range sessions {
		if sessions[i].Name != name {
			continue
		}
		if ok, reason := sessions[i].CanReceive(mode); !ok {
			return fmt.Errorf("not sending to %s: %s", name, reason)
		}
		if err := client.SendText(ctx, name, text); err != nil {
			return err
		}
		fmt.Printf("Sent to %s\n", name)
		return nil
	}
	return fmt.Errorf("no session named %q", name)
}

// runReply answers sessions that are asking a question:
// `reply <session> <text>` for one, `reply --asking [--yes] <text>` for all.
func runReply(args []string) error {
	fs := newFlagSet("reply")
	all := fs.Bool("asking", false, "answer every session that is asking a question")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("reply: %w", err)
	}
	if !*all {
		if fs.NArg() < 2 {
			return errors.New("usage: claude-dashboard reply <session> <text> | reply --asking [--yes] <text>")
		}
		return sendOne(fs.Arg(0), strings.Join(fs.Args()[1:], " "), session.ModeReply)
	}

	text := strings.Join(fs.Args(), " ")
	if text == "" {
		return errors.New("usage: claude-dashboard reply --asking [--yes] <text>")
	}
	if err := tmux.ValidateSendText(text); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, sessions, err := detect(ctx)
	if err != nil {
		return err
	}
	var targets []string
	for i := range sessions {
		if ok, _ := sessions[i].CanReceive(session.ModeReply); ok {
			targets = append(targets, sessions[i].Name)
		}
	}
	sort.Strings(targets)
	if len(targets) == 0 {
		fmt.Println("No session is asking a question.")
		return nil
	}
	fmt.Printf("Reply %q to %d session(s): %s\n", truncate(text, 80), len(targets), strings.Join(targets, ", "))
	if !*yes {
		if !stdinIsTerminal() {
			return errors.New("refusing to broadcast without a terminal to confirm on; pass --yes")
		}
		if !confirm("Send?") {
			fmt.Println("Cancelled.")
			return nil
		}
	}
	var failed []string
	for _, name := range targets {
		if err := client.SendText(ctx, name, text); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		fmt.Printf("Sent to %s\n", name)
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d failed:\n  %s", len(failed), strings.Join(failed, "\n  "))
	}
	return nil
}

// ------------------------------------------------------------ dispatch

// runDispatch hands queued tasks to idle sessions. The target sessions must
// be named (--sessions) or explicitly widened (--all): it never defaults to
// every session.
func runDispatch(args []string) error {
	fs := newFlagSet("dispatch")
	sessionsFlag := fs.String("sessions", "", "comma-separated session names or globs, e.g. a,b,c or 'work-*'")
	all := fs.Bool("all", false, "every managed session")
	behind := fs.Bool("queue-behind-work", false, "also give tasks to busy sessions")
	maxActive := fs.Int("max-active", 0, "do not start tasks while this many matched sessions are active (0 = no limit)")
	dry := fs.Bool("dry-run", false, "show what would be sent without sending")
	watch := fs.Bool("watch", false, "keep dispatching until interrupted")
	interval := fs.Duration("interval", 10*time.Second, "time between passes with --watch")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("dispatch: %w", err)
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("dispatch: unexpected argument %q", fs.Arg(0))
	}
	if *all == (*sessionsFlag != "") {
		return errors.New("choose exactly one of --sessions <names> or --all")
	}
	if *maxActive < 0 {
		return errors.New("--max-active must not be negative")
	}
	if *watch && *dry {
		return errors.New("--dry-run cannot be combined with --watch")
	}
	if *watch && *interval < minWatchInterval {
		return fmt.Errorf("--interval must be at least %s", minWatchInterval)
	}

	globs := []string{"*"}
	if !*all {
		globs = globs[:0]
		for _, g := range strings.Split(*sessionsFlag, ",") {
			if g = strings.TrimSpace(g); g != "" {
				globs = append(globs, g)
			}
		}
	}
	mode := session.ModeIdleOnly
	if *behind {
		mode = session.ModeQueueBehindWork
	}

	client, err := tmux.NewClient()
	if err != nil {
		return err
	}
	q := queue.Open(queue.DefaultPath())
	det := session.NewDetector(client)
	opt := dispatch.Options{Sessions: globs, Mode: mode, MaxActive: *maxActive, DryRun: *dry}

	if *watch {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		fmt.Printf("Dispatching to %s every %s. Ctrl+C to stop.\n", strings.Join(globs, ","), *interval)
		return dispatch.Watch(ctx, q, det, client, issue.OriginRepo, opt, *interval, func(r dispatch.Report) {
			printReport(r, false)
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	rpt, err := dispatch.Once(ctx, q, det, client, issue.OriginRepo, opt)
	if err != nil {
		return err
	}
	if *dry {
		fmt.Println("Dry run: nothing was sent.")
	}
	printReport(rpt, true)
	return nil
}

// printReport prints what a pass did. Skipped sessions are listed only when
// verbose, so a watching dispatcher stays quiet between events.
func printReport(r dispatch.Report, verbose bool) {
	stamp := time.Now().Format("15:04:05")
	for _, s := range r.Sent {
		what := s.Task.Ref
		if what == "" {
			what = truncate(s.Task.Text, 60)
		}
		fmt.Printf("%s  -> %s  #%d %s\n", stamp, s.Session, s.Task.ID, what)
	}
	for _, err := range r.Failed {
		fmt.Fprintf(os.Stderr, "%s  ! %v\n", stamp, err)
	}
	for _, t := range r.Unmatched {
		fmt.Fprintf(os.Stderr, "%s  ? task %d needs a clone of %s but no selected session has one\n", stamp, t.ID, t.Repo)
	}
	if verbose {
		names := make([]string, 0, len(r.Skipped))
		for name := range r.Skipped {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Printf("%s  - %s skipped: %s\n", stamp, name, r.Skipped[name])
		}
		if len(r.Sent) == 0 && len(r.Failed) == 0 && len(names) == 0 {
			fmt.Println("Nothing to do.")
		}
	}
}
