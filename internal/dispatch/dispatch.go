// Package dispatch sends queued tasks to available Claude Code sessions.
package dispatch

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"time"

	"github.com/seunggabi/claude-dashboard/internal/queue"
	"github.com/seunggabi/claude-dashboard/internal/session"
	"github.com/seunggabi/claude-dashboard/internal/tmux"
)

// Detector discovers running sessions.
type Detector interface {
	Detect(ctx context.Context) ([]session.Session, error)
}

// Sender types a line of text into a named session and presses Enter.
type Sender interface {
	SendText(ctx context.Context, name, text string) error
}

// RepoResolver returns "owner/name" for the git origin of dir, or "" when
// unknown. It may be called multiple times for the same dir; results are
// memoised inside a single Once call.
type RepoResolver func(dir string) string

// Options controls a single dispatch pass.
type Options struct {
	// Sessions is a list of path.Match globs matched against session names.
	// REQUIRED: Once returns an error if the slice is empty.
	Sessions []string
	// Mode is forwarded to session.CanReceive.
	Mode session.SendMode
	// MaxActive: do not send when this many matched sessions are already
	// StatusActive. 0 means unlimited. A session that was StatusIdle and
	// received a task counts as active for the remainder of the same pass.
	MaxActive int
	// DryRun previews dispatch decisions without claiming tasks or sending.
	DryRun bool
	// Accept is an optional extra filter applied to each pending task before
	// dispatch. A task must pass both the repo-match check and Accept to be
	// eligible. nil means accept all tasks.
	Accept func(queue.Task) bool
}

// Sent records one task dispatched to one session.
type Sent struct {
	Session string
	Task    queue.Task
}

// Report summarises what Once decided and did.
type Report struct {
	Sent      []Sent
	Skipped   map[string]string // session name → reason skipped
	Failed    []error           // errors from Claim or SendText
	Unmatched []queue.Task      // pending tasks whose Repo no matched session can serve
	Held      []queue.Task      // pending tasks rejected by Options.Accept (not repo-mismatched)
}

// Once runs one dispatch pass: for each eligible managed session (in name
// order) it sends at most one pending task from q.
func Once(ctx context.Context, q *queue.Queue, det Detector, snd Sender, resolve RepoResolver, opt Options) (Report, error) {
	if len(opt.Sessions) == 0 {
		return Report{}, fmt.Errorf("dispatch: Sessions glob list must not be empty")
	}

	sessions, err := det.Detect(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("dispatch: detect sessions: %w", err)
	}

	// Keep only Managed sessions whose name matches at least one glob.
	var matched []session.Session
	for _, s := range sessions {
		if !s.Managed {
			continue
		}
		for _, glob := range opt.Sessions {
			if ok, _ := path.Match(glob, s.Name); ok {
				matched = append(matched, s)
				break
			}
		}
	}

	// Deterministic order: sort by name.
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Name < matched[j].Name
	})

	// Load all tasks once per pass for last-dispatch computation and Unmatched.
	allTasks, err := q.List()
	if err != nil {
		return Report{}, fmt.Errorf("dispatch: list tasks: %w", err)
	}

	// lastDispatch maps each session name to the most recent DispatchedAt among
	// its currently-dispatched tasks. Used to guard against double-dispatch.
	lastDispatch := computeLastDispatch(allTasks)

	// pending is the ordered subset that is still waiting.
	pending := filterPending(allTasks)

	// pathCache memoises RepoResolver results for this pass.
	pathCache := make(map[string]string)

	// Compute Unmatched: pending tasks with Repo set for which no matched
	// session can resolve to that repo.
	unmatched := computeUnmatched(pending, matched, resolve, pathCache)

	// Compute Held: pending tasks rejected by opt.Accept that are not already
	// blocked by repo mismatch (those are reported as Unmatched instead).
	held := computeHeld(pending, unmatched, opt.Accept)

	// Seed active count from sessions already StatusActive.
	activeCount := 0
	for _, s := range matched {
		if s.Status == session.StatusActive {
			activeCount++
		}
	}

	rpt := Report{
		Skipped:   make(map[string]string),
		Unmatched: unmatched,
		Held:      held,
	}

	if opt.DryRun {
		return onceDryRun(pending, matched, lastDispatch, resolve, opt, rpt, activeCount, pathCache)
	}
	return onceLive(ctx, q, snd, matched, lastDispatch, resolve, opt, rpt, activeCount, pathCache)
}

// onceDryRun previews what Once would do without touching the queue.
func onceDryRun(
	pending []queue.Task,
	matched []session.Session,
	lastDispatch map[string]time.Time,
	resolve RepoResolver,
	opt Options,
	rpt Report,
	activeCount int,
	pathCache map[string]string,
) (Report, error) {
	taken := make(map[int]bool) // virtually claimed task IDs this pass

	for i := range matched {
		s := matched[i]
		if opt.MaxActive > 0 && activeCount >= opt.MaxActive {
			rpt.Skipped[s.Name] = fmt.Sprintf("max-active limit (%d) reached", opt.MaxActive)
			continue
		}
		ok, reason := s.CanReceive(opt.Mode)
		if !ok {
			rpt.Skipped[s.Name] = reason
			continue
		}
		if skip, reason := hookAtSkipReason(s, lastDispatch); skip {
			rpt.Skipped[s.Name] = reason
			continue
		}

		var chosen *queue.Task
		for j := range pending {
			t := pending[j]
			if taken[t.ID] {
				continue
			}
			if taskFits(t, s, resolve, pathCache) && (opt.Accept == nil || opt.Accept(t)) {
				chosen = &pending[j]
				break
			}
		}
		if chosen == nil {
			rpt.Skipped[s.Name] = "no fitting task"
			continue
		}

		taken[chosen.ID] = true
		rpt.Sent = append(rpt.Sent, Sent{Session: s.Name, Task: *chosen})
		if s.Status == session.StatusIdle {
			activeCount++
		}
	}
	return rpt, nil
}

// onceLive performs the real dispatch: Claim → SendText → Release on failure.
func onceLive(
	ctx context.Context,
	q *queue.Queue,
	snd Sender,
	matched []session.Session,
	lastDispatch map[string]time.Time,
	resolve RepoResolver,
	opt Options,
	rpt Report,
	activeCount int,
	pathCache map[string]string,
) (Report, error) {
	for i := range matched {
		s := matched[i]
		if opt.MaxActive > 0 && activeCount >= opt.MaxActive {
			rpt.Skipped[s.Name] = fmt.Sprintf("max-active limit (%d) reached", opt.MaxActive)
			continue
		}
		ok, reason := s.CanReceive(opt.Mode)
		if !ok {
			rpt.Skipped[s.Name] = reason
			continue
		}
		if skip, reason := hookAtSkipReason(s, lastDispatch); skip {
			rpt.Skipped[s.Name] = reason
			continue
		}

		accept := makeAccept(s, resolve, pathCache, opt.Accept)

		claimed, err := q.Claim(s.Name, accept)
		if err != nil {
			rpt.Failed = append(rpt.Failed, fmt.Errorf("claim for session %s: %w", s.Name, err))
			continue
		}
		if claimed == nil {
			rpt.Skipped[s.Name] = "no fitting task"
			continue
		}

		if err := snd.SendText(ctx, s.Name, claimed.Text); err != nil {
			if errors.Is(err, tmux.ErrTypedNotSubmitted) {
				// Text reached the pane but Enter was never pressed: the task
				// is in an ambiguous state. Keep it Dispatched and alert the
				// operator; DO NOT Release (that would re-queue a command
				// already visible in the pane).
				rpt.Failed = append(rpt.Failed, fmt.Errorf(
					"send to %s: text is unsubmitted in session and needs manual review: %w",
					s.Name, err))
			} else {
				// Clean failure (nothing was typed): Release so it can be
				// retried by the next pass.
				rpt.Failed = append(rpt.Failed, fmt.Errorf("send to %s: %w", s.Name, err))
				if relErr := q.Release(claimed.ID); relErr != nil {
					rpt.Failed = append(rpt.Failed, fmt.Errorf(
						"release task %d after send failure: %w", claimed.ID, relErr))
				}
			}
			continue
		}

		rpt.Sent = append(rpt.Sent, Sent{Session: s.Name, Task: *claimed})
		if s.Status == session.StatusIdle {
			activeCount++
		}
	}
	return rpt, nil
}

// hookAtSkipReason returns (true, reason) when the session's last hook report
// predates or equals the most recent dispatch to it, indicating a potential
// double-dispatch window. It returns (false, "") when the session is safe to
// consider.
//
// The comparison is plain HookAt.After(lastDispatch[name]): hooks are sampled
// at second resolution, so a hook in the same second as the dispatch is NOT
// considered newer and correctly causes a skip.
func hookAtSkipReason(s session.Session, lastDispatch map[string]time.Time) (bool, string) {
	if s.HookAt.IsZero() {
		return true, "no hook report since last dispatch"
	}
	ld := lastDispatch[s.Name]
	if !ld.IsZero() && !s.HookAt.After(ld) {
		return true, "no hook report since last dispatch"
	}
	return false, ""
}

// computeLastDispatch returns the most recent DispatchedAt for each session
// that currently owns a Dispatched task.
func computeLastDispatch(tasks []queue.Task) map[string]time.Time {
	m := make(map[string]time.Time)
	for _, t := range tasks {
		if t.Status != queue.Dispatched || t.Session == "" {
			continue
		}
		if t.DispatchedAt.After(m[t.Session]) {
			m[t.Session] = t.DispatchedAt
		}
	}
	return m
}

// filterPending returns the subset of tasks that are still Pending.
func filterPending(tasks []queue.Task) []queue.Task {
	var out []queue.Task
	for _, t := range tasks {
		if t.Status == queue.Pending {
			out = append(out, t)
		}
	}
	return out
}

// computeUnmatched returns pending tasks whose Repo field cannot be served by
// any of the matched sessions (i.e. none of them resolves to that repo).
// Tasks with an empty Repo are never included (they fit any session).
func computeUnmatched(pending []queue.Task, matched []session.Session, resolve RepoResolver, pathCache map[string]string) []queue.Task {
	var out []queue.Task
	for _, t := range pending {
		if t.Repo == "" {
			continue
		}
		served := false
		for _, s := range matched {
			if taskFits(t, s, resolve, pathCache) {
				served = true
				break
			}
		}
		if !served {
			out = append(out, t)
		}
	}
	return out
}

// makeAccept returns an accept function for q.Claim that checks repo matching
// and the optional caller-supplied Accept filter. A task must pass both.
func makeAccept(s session.Session, resolve RepoResolver, pathCache map[string]string, accept func(queue.Task) bool) func(queue.Task) bool {
	return func(t queue.Task) bool {
		if !taskFits(t, s, resolve, pathCache) {
			return false
		}
		return accept == nil || accept(t)
	}
}

// computeHeld returns pending tasks that are rejected by the Accept filter and
// are not already counted as Unmatched (repo-blocked). Tasks are returned in
// queue order. If accept is nil, no tasks are held.
func computeHeld(pending []queue.Task, unmatched []queue.Task, accept func(queue.Task) bool) []queue.Task {
	if accept == nil {
		return nil
	}
	unmatchedIDs := make(map[int]bool, len(unmatched))
	for _, t := range unmatched {
		unmatchedIDs[t.ID] = true
	}
	var out []queue.Task
	for _, t := range pending {
		if unmatchedIDs[t.ID] {
			continue // blocked by repo mismatch, not by Accept
		}
		if !accept(t) {
			out = append(out, t)
		}
	}
	return out
}

// taskFits reports whether a task may be sent to s. A task with an empty Repo
// fits any session. When Repo is set, resolve is called (at most once per path)
// to compare the session's checked-out repository.
func taskFits(t queue.Task, s session.Session, resolve RepoResolver, pathCache map[string]string) bool {
	if t.Repo == "" {
		return true
	}
	if resolve == nil {
		return false
	}
	cached, ok := pathCache[s.Path]
	if !ok {
		cached = resolve(s.Path)
		pathCache[s.Path] = cached
	}
	return cached == t.Repo
}

// Watch runs Once immediately at t=0 and then again every 'every' until ctx
// is cancelled. Transient errors (from Detect or Claim) are appended to
// Report.Failed and delivered to onReport; they do not stop the loop.
// Watch returns only when ctx is done or the initial validation fails.
func Watch(
	ctx context.Context,
	q *queue.Queue,
	det Detector,
	snd Sender,
	resolve RepoResolver,
	opt Options,
	every time.Duration,
	onReport func(Report),
) error {
	if len(opt.Sessions) == 0 {
		return fmt.Errorf("dispatch: Sessions glob list must not be empty")
	}

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		// Run a pass immediately (first at t=0, then after each tick).
		rpt, err := Once(ctx, q, det, snd, resolve, opt)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			rpt.Failed = append(rpt.Failed, err)
		}
		if onReport != nil {
			onReport(rpt)
		}

		// Wait for the next tick or cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
