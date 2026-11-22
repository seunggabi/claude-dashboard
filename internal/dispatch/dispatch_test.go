package dispatch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/seunggabi/claude-dashboard/internal/queue"
	"github.com/seunggabi/claude-dashboard/internal/session"
	"github.com/seunggabi/claude-dashboard/internal/tmux"
)

// ---- Fakes -----------------------------------------------------------------

type fakeDetector struct {
	sessions []session.Session
	err      error
}

func (f *fakeDetector) Detect(_ context.Context) ([]session.Session, error) {
	return f.sessions, f.err
}

type fakeSender struct {
	sent []sentRecord
	// errFor maps session name to the error to return for that session.
	errFor map[string]error
}

type sentRecord struct {
	name string
	text string
}

func (f *fakeSender) SendText(_ context.Context, name, text string) error {
	if f.errFor != nil {
		if err, ok := f.errFor[name]; ok {
			return err
		}
	}
	f.sent = append(f.sent, sentRecord{name: name, text: text})
	return nil
}

// idleSession builds a fully-eligible managed idle session.
// HookAt is set to time.Now() so the new hookAtSkipReason check passes
// for sessions with no prior dispatch (lastDispatch is zero time, and
// any non-zero HookAt is After zero time).
func idleSession(name, path string) session.Session {
	return session.Session{
		Name:    name,
		Managed: true,
		Status:  session.StatusIdle,
		Hook:    "idle",
		HookAt:  time.Now(),
		Path:    path,
	}
}

func activeSession(name, path string) session.Session {
	return session.Session{
		Name:    name,
		Managed: true,
		Status:  session.StatusActive,
		Hook:    "active",
		HookAt:  time.Now(),
		Path:    path,
	}
}

func newTestQ(t *testing.T) *queue.Queue {
	t.Helper()
	return queue.Open(filepath.Join(t.TempDir(), "q.json"))
}

// ---- Once: validation ------------------------------------------------------

func TestOnce_EmptySessions_Error(t *testing.T) {
	q := newTestQ(t)
	det := &fakeDetector{}
	snd := &fakeSender{}
	_, err := Once(context.Background(), q, det, snd, nil, Options{})
	if err == nil {
		t.Error("Once with empty Sessions: want error, got nil")
	}
}

// ---- Once: glob filtering --------------------------------------------------

func TestOnce_GlobFiltering(t *testing.T) {
	q := newTestQ(t)
	q.Add("task1", "", "") //nolint:errcheck

	det := &fakeDetector{sessions: []session.Session{
		idleSession("foo-1", "/foo"),
		idleSession("bar-1", "/bar"),
		idleSession("foo-2", "/foo2"),
	}}
	snd := &fakeSender{}

	opt := Options{Sessions: []string{"foo-*"}, Mode: session.ModeIdleOnly}
	rpt, err := Once(context.Background(), q, det, snd, nil, opt)
	if err != nil {
		t.Fatalf("Once: %v", err)
	}

	// Only foo-1 should have been sent to (foo-2 also matches but only one task).
	for _, s := range rpt.Sent {
		if s.Session != "foo-1" && s.Session != "foo-2" {
			t.Errorf("sent to non-matching session %q", s.Session)
		}
	}
	// bar-1 must not appear in Sent.
	for _, s := range rpt.Sent {
		if s.Session == "bar-1" {
			t.Error("bar-1 should not have received a task (not matching glob foo-*)")
		}
	}
	if len(rpt.Sent) != 1 {
		t.Errorf("Sent = %d, want 1 (only 1 task queued)", len(rpt.Sent))
	}
}

// ---- Once: CanReceive enforcement ------------------------------------------

func TestOnce_WaitingSession_Skipped(t *testing.T) {
	q := newTestQ(t)
	q.Add("task", "", "") //nolint:errcheck

	det := &fakeDetector{sessions: []session.Session{{
		Name:    "s1",
		Managed: true,
		Status:  session.StatusWaiting,
		Hook:    "waiting",
	}}}
	snd := &fakeSender{}

	rpt, err := Once(context.Background(), q, det, snd, nil, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 0 {
		t.Errorf("waiting session: Sent = %d, want 0", len(rpt.Sent))
	}
	if _, skipped := rpt.Skipped["s1"]; !skipped {
		t.Error("waiting session should appear in Skipped")
	}
}

func TestOnce_UnmanagedSession_Skipped(t *testing.T) {
	q := newTestQ(t)
	q.Add("task", "", "") //nolint:errcheck

	det := &fakeDetector{sessions: []session.Session{{
		Name:    "s1",
		Managed: false, // terminal process, not a tmux session
		Status:  session.StatusIdle,
		Hook:    "idle",
	}}}
	snd := &fakeSender{}

	rpt, err := Once(context.Background(), q, det, snd, nil, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 0 {
		t.Errorf("unmanaged session: Sent = %d, want 0", len(rpt.Sent))
	}
}

func TestOnce_NoHookSession_Skipped(t *testing.T) {
	q := newTestQ(t)
	q.Add("task", "", "") //nolint:errcheck

	// StatusIdle but Hook="" means no hook report — CanReceive returns false.
	det := &fakeDetector{sessions: []session.Session{{
		Name:    "s1",
		Managed: true,
		Status:  session.StatusIdle,
		Hook:    "", // no hook
	}}}
	snd := &fakeSender{}

	rpt, err := Once(context.Background(), q, det, snd, nil, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 0 {
		t.Errorf("no-hook idle session: Sent = %d, want 0", len(rpt.Sent))
	}
	if _, skipped := rpt.Skipped["s1"]; !skipped {
		t.Error("no-hook session should appear in Skipped")
	}
}

// ---- Once: one task per session per pass -----------------------------------

func TestOnce_OneTaskPerSession(t *testing.T) {
	q := newTestQ(t)
	q.Add("task1", "", "") //nolint:errcheck
	q.Add("task2", "", "") //nolint:errcheck
	q.Add("task3", "", "") //nolint:errcheck

	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/"),
	}}
	snd := &fakeSender{}

	rpt, err := Once(context.Background(), q, det, snd, nil, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 1 {
		t.Errorf("Sent = %d, want 1 (at most one task per session)", len(rpt.Sent))
	}
}

// ---- Once: repo matching ---------------------------------------------------

func TestOnce_RepoMatch(t *testing.T) {
	q := newTestQ(t)
	q.Add("for a", "owner/a", "") //nolint:errcheck
	q.Add("for b", "owner/b", "") //nolint:errcheck

	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/path/a"),
		idleSession("s2", "/path/b"),
	}}
	snd := &fakeSender{}

	resolve := func(dir string) string {
		switch dir {
		case "/path/a":
			return "owner/a"
		case "/path/b":
			return "owner/b"
		}
		return ""
	}

	rpt, err := Once(context.Background(), q, det, snd, resolve, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 2 {
		t.Fatalf("Sent = %d, want 2", len(rpt.Sent))
	}

	sentMap := make(map[string]string) // session → task text
	for _, s := range rpt.Sent {
		sentMap[s.Session] = s.Task.Text
	}
	if sentMap["s1"] != "for a" {
		t.Errorf("s1 got %q, want 'for a'", sentMap["s1"])
	}
	if sentMap["s2"] != "for b" {
		t.Errorf("s2 got %q, want 'for b'", sentMap["s2"])
	}
}

func TestOnce_RepoMismatch_Skipped(t *testing.T) {
	q := newTestQ(t)
	q.Add("for x", "owner/x", "") //nolint:errcheck

	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/path/y"),
	}}
	snd := &fakeSender{}

	resolve := func(dir string) string { return "owner/y" }

	rpt, err := Once(context.Background(), q, det, snd, resolve, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 0 {
		t.Errorf("repo mismatch: Sent = %d, want 0", len(rpt.Sent))
	}
	if _, skipped := rpt.Skipped["s1"]; !skipped {
		t.Error("repo-mismatch session should be in Skipped")
	}
}

func TestOnce_EmptyRepoTask_AnySession(t *testing.T) {
	q := newTestQ(t)
	q.Add("any session", "", "") //nolint:errcheck

	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/path/a"),
	}}
	snd := &fakeSender{}

	// resolve should NOT be called for a task with empty Repo.
	called := false
	resolve := func(dir string) string { called = true; return "some/repo" }

	rpt, err := Once(context.Background(), q, det, snd, resolve, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 1 {
		t.Errorf("empty-repo task: Sent = %d, want 1", len(rpt.Sent))
	}
	if called {
		t.Error("resolve must not be called when task.Repo is empty")
	}
}

// ---- Once: MaxActive -------------------------------------------------------

func TestOnce_MaxActive_LimitsNewDispatches(t *testing.T) {
	q := newTestQ(t)
	q.Add("t1", "", "") //nolint:errcheck
	q.Add("t2", "", "") //nolint:errcheck
	q.Add("t3", "", "") //nolint:errcheck

	// Three idle sessions; MaxActive=1 → only the first idle that receives a
	// task counts as active, so the second and third are blocked.
	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/"),
		idleSession("s2", "/"),
		idleSession("s3", "/"),
	}}
	snd := &fakeSender{}

	opt := Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly, MaxActive: 1}
	rpt, err := Once(context.Background(), q, det, snd, nil, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 1 {
		t.Errorf("MaxActive=1: Sent = %d, want 1", len(rpt.Sent))
	}
}

func TestOnce_MaxActive_AlreadyActive(t *testing.T) {
	q := newTestQ(t)
	q.Add("task", "", "") //nolint:errcheck

	// Two already-active sessions; MaxActive=2 → limit already reached.
	det := &fakeDetector{sessions: []session.Session{
		activeSession("s1", "/"),
		activeSession("s2", "/"),
		idleSession("s3", "/"),
	}}
	snd := &fakeSender{}

	opt := Options{Sessions: []string{"*"}, Mode: session.ModeQueueBehindWork, MaxActive: 2}
	rpt, err := Once(context.Background(), q, det, snd, nil, opt)
	if err != nil {
		t.Fatal(err)
	}
	// s1 and s2 are skipped because they already count toward the max (they'd be
	// handled first alphabetically); s3 is then at the limit.
	// No task sent regardless.
	if len(rpt.Sent) != 0 {
		t.Errorf("MaxActive=2 with 2 already active: Sent = %d, want 0", len(rpt.Sent))
	}
}

// ---- Once: DryRun ----------------------------------------------------------

func TestOnce_DryRun_QueueUntouched(t *testing.T) {
	q := newTestQ(t)
	q.Add("t1", "", "") //nolint:errcheck
	q.Add("t2", "", "") //nolint:errcheck

	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/"),
		idleSession("s2", "/"),
	}}
	snd := &fakeSender{}

	opt := Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly, DryRun: true}
	rpt, err := Once(context.Background(), q, det, snd, nil, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 2 {
		t.Errorf("DryRun Sent = %d, want 2", len(rpt.Sent))
	}

	// Queue must be unchanged.
	pending, err := q.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Errorf("after DryRun, pending = %d, want 2 (queue must be untouched)", len(pending))
	}
	// Sender must NOT have been called.
	if len(snd.sent) != 0 {
		t.Errorf("DryRun: sender was called %d times, want 0", len(snd.sent))
	}
}

// ---- Once: send failure releases task --------------------------------------

func TestOnce_SendFailure_ReleasesTask(t *testing.T) {
	q := newTestQ(t)
	q.Add("hello", "", "") //nolint:errcheck

	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/"),
	}}
	snd := &fakeSender{errFor: map[string]error{
		"s1": errors.New("tmux gone"),
	}}

	opt := Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly}
	rpt, err := Once(context.Background(), q, det, snd, nil, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 0 {
		t.Errorf("after send failure: Sent = %d, want 0", len(rpt.Sent))
	}
	if len(rpt.Failed) == 0 {
		t.Error("after send failure: Failed should be non-empty")
	}

	// Task must be back in Pending.
	pending, err := q.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Errorf("after send failure, pending = %d, want 1 (task should be released)", len(pending))
	}
}

// ---- Once: FIFO order across sessions --------------------------------------

func TestOnce_FIFO_AcrossSessions(t *testing.T) {
	q := newTestQ(t)
	texts := []string{"t1", "t2", "t3"}
	for _, text := range texts {
		q.Add(text, "", "") //nolint:errcheck
	}

	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/"),
		idleSession("s2", "/"),
		idleSession("s3", "/"),
	}}
	snd := &fakeSender{}

	opt := Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly}
	rpt, err := Once(context.Background(), q, det, snd, nil, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 3 {
		t.Fatalf("Sent = %d, want 3", len(rpt.Sent))
	}

	// Sessions are sorted by name: s1 < s2 < s3.
	// FIFO: s1 gets t1, s2 gets t2, s3 gets t3.
	want := []struct {
		sess string
		text string
	}{
		{"s1", "t1"},
		{"s2", "t2"},
		{"s3", "t3"},
	}
	for i, w := range want {
		if rpt.Sent[i].Session != w.sess {
			t.Errorf("Sent[%d].Session = %q, want %q", i, rpt.Sent[i].Session, w.sess)
		}
		if rpt.Sent[i].Task.Text != w.text {
			t.Errorf("Sent[%d].Task.Text = %q, want %q", i, rpt.Sent[i].Task.Text, w.text)
		}
	}
}

// ---- RepoResolver caching --------------------------------------------------

func TestOnce_ResolverCachedPerPath(t *testing.T) {
	q := newTestQ(t)
	q.Add("t1", "owner/repo", "") //nolint:errcheck
	q.Add("t2", "owner/repo", "") //nolint:errcheck

	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/shared"),
		idleSession("s2", "/shared"), // same path as s1
	}}
	snd := &fakeSender{}

	callCount := 0
	resolve := func(dir string) string {
		callCount++
		return "owner/repo"
	}

	opt := Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly}
	_, err := Once(context.Background(), q, det, snd, resolve, opt)
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Errorf("resolve called %d times for same path, want 1 (should be cached)", callCount)
	}
}

// ---- Once: duplicate-dispatch guard (HookAt checks) -----------------------

// TestOnce_HookAtZero_Skipped verifies that a session with Hook="idle"
// (which passes CanReceive) but HookAt=zero is still skipped, because a
// zero HookAt means no hook report has been recorded.
func TestOnce_HookAtZero_Skipped(t *testing.T) {
	q := newTestQ(t)
	q.Add("task", "", "")

	det := &fakeDetector{sessions: []session.Session{{
		Name:    "s1",
		Managed: true,
		Status:  session.StatusIdle,
		Hook:    "idle",
		HookAt:  time.Time{}, // zero — no hook report recorded
	}}}
	snd := &fakeSender{}

	rpt, err := Once(context.Background(), q, det, snd, nil, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 0 {
		t.Errorf("HookAt zero: Sent = %d, want 0", len(rpt.Sent))
	}
	if rpt.Skipped["s1"] != "no hook report since last dispatch" {
		t.Errorf("wrong skip reason: %q", rpt.Skipped["s1"])
	}
}

// TestOnce_HookAt_SameSecond_NotEligible verifies the duplicate-dispatch
// guard: a hook report in the same second as the previous dispatch is NOT
// considered newer (HookAt.After(DispatchedAt) is false when both truncate
// to the same second).
func TestOnce_HookAt_SameSecond_NotEligible(t *testing.T) {
	q := newTestQ(t)
	q.Add("prior", "", "")

	// Claim "prior" to record a DispatchedAt timestamp for s1.
	prior, err := q.Claim("s1", nil)
	if err != nil || prior == nil {
		t.Fatal("setup: failed to claim prior task")
	}

	// HookAt = DispatchedAt truncated to the second.
	// Because time.Now() has sub-second precision, Truncate(second) ≤ original,
	// so hookAt.After(prior.DispatchedAt) is false.
	hookAt := prior.DispatchedAt.Truncate(time.Second)

	q.Add("new task", "", "")

	det := &fakeDetector{sessions: []session.Session{{
		Name:    "s1",
		Managed: true,
		Status:  session.StatusIdle,
		Hook:    "idle",
		HookAt:  hookAt,
	}}}
	snd := &fakeSender{}

	rpt, err := Once(context.Background(), q, det, snd, nil, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 0 {
		t.Errorf("same-second hook: Sent = %d, want 0 (hook in same second must not count)", len(rpt.Sent))
	}
	if rpt.Skipped["s1"] != "no hook report since last dispatch" {
		t.Errorf("wrong skip reason: %q", rpt.Skipped["s1"])
	}
}

// TestOnce_HookAt_AfterDispatch_Eligible verifies that a hook report
// strictly after the last DispatchedAt allows the session to receive tasks.
func TestOnce_HookAt_AfterDispatch_Eligible(t *testing.T) {
	q := newTestQ(t)
	q.Add("prior", "", "")

	prior, err := q.Claim("s1", nil)
	if err != nil || prior == nil {
		t.Fatal("setup: failed to claim prior task")
	}

	// HookAt one second after DispatchedAt → strictly newer → eligible.
	hookAt := prior.DispatchedAt.Add(time.Second)

	q.Add("new task", "", "")

	det := &fakeDetector{sessions: []session.Session{{
		Name:    "s1",
		Managed: true,
		Status:  session.StatusIdle,
		Hook:    "idle",
		HookAt:  hookAt,
	}}}
	snd := &fakeSender{}

	rpt, err := Once(context.Background(), q, det, snd, nil, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 1 {
		t.Errorf("hook after dispatch: Sent = %d, want 1", len(rpt.Sent))
	}
}

// ---- Once: ErrTypedNotSubmitted keeps task dispatched ----------------------

// TestOnce_TypedNotSubmitted_WrappedError verifies that a wrapped
// tmux.ErrTypedNotSubmitted does NOT release the task: the text is already
// visible in the pane and re-queueing it would duplicate the command.
func TestOnce_TypedNotSubmitted_WrappedError(t *testing.T) {
	q := newTestQ(t)
	q.Add("hello", "", "")

	snd := &fakeSender{errFor: map[string]error{
		"s1": fmt.Errorf("enter key failed: %w", tmux.ErrTypedNotSubmitted),
	}}
	det := &fakeDetector{sessions: []session.Session{idleSession("s1", "/")}}

	rpt, err := Once(context.Background(), q, det, snd, nil, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 0 {
		t.Errorf("ErrTypedNotSubmitted (wrapped): Sent = %d, want 0", len(rpt.Sent))
	}
	if len(rpt.Failed) == 0 {
		t.Error("ErrTypedNotSubmitted (wrapped): Failed should be non-empty")
	}

	// Task must stay Dispatched — NOT released back to pending.
	pending, err := q.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("after ErrTypedNotSubmitted: pending = %d, want 0 (task stays dispatched)", len(pending))
	}
	all, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Status != queue.Dispatched {
		t.Errorf("after ErrTypedNotSubmitted: status = %v, want Dispatched", all[0].Status)
	}
}

// TestOnce_TypedNotSubmitted_PlainError verifies that a plain (unwrapped)
// tmux.ErrTypedNotSubmitted is also caught by errors.Is and keeps the task
// dispatched.
func TestOnce_TypedNotSubmitted_PlainError(t *testing.T) {
	q := newTestQ(t)
	q.Add("hello", "", "")

	snd := &fakeSender{errFor: map[string]error{
		"s1": tmux.ErrTypedNotSubmitted, // plain, not wrapped
	}}
	det := &fakeDetector{sessions: []session.Session{idleSession("s1", "/")}}

	_, err := Once(context.Background(), q, det, snd, nil, Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly})
	if err != nil {
		t.Fatal(err)
	}

	// Task must stay Dispatched.
	pending, _ := q.Pending()
	if len(pending) != 0 {
		t.Errorf("plain ErrTypedNotSubmitted: pending = %d, want 0 (task stays dispatched)", len(pending))
	}
	all, _ := q.List()
	if len(all) != 1 || all[0].Status != queue.Dispatched {
		t.Errorf("plain ErrTypedNotSubmitted: status = %v, want Dispatched", all[0].Status)
	}
}

// ---- Once: Unmatched field -------------------------------------------------

// TestOnce_Unmatched verifies that Report.Unmatched contains pending tasks
// whose Repo cannot be served by any matched session, while tasks with an
// empty Repo (which fit any session) are never included.
func TestOnce_Unmatched(t *testing.T) {
	q := newTestQ(t)
	q.Add("any", "", "")          // Repo="" → never unmatched
	q.Add("for x", "owner/x", "") // No session resolves to owner/x → unmatched
	q.Add("for y", "owner/y", "") // s1 at /path/y resolves to owner/y → NOT unmatched

	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/path/y"),
	}}
	snd := &fakeSender{}
	resolve := func(dir string) string {
		if dir == "/path/y" {
			return "owner/y"
		}
		return ""
	}

	opt := Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly}
	rpt, err := Once(context.Background(), q, det, snd, resolve, opt)
	if err != nil {
		t.Fatal(err)
	}

	if len(rpt.Unmatched) != 1 {
		t.Fatalf("Unmatched = %d, want 1; unmatched = %v", len(rpt.Unmatched), rpt.Unmatched)
	}
	if rpt.Unmatched[0].Repo != "owner/x" {
		t.Errorf("Unmatched[0].Repo = %q, want owner/x", rpt.Unmatched[0].Repo)
	}
}

// TestOnce_DryRun_Unmatched verifies that Unmatched is computed correctly in
// DryRun mode.
func TestOnce_DryRun_Unmatched(t *testing.T) {
	q := newTestQ(t)
	q.Add("for z", "owner/z", "") // No session resolves to owner/z → unmatched

	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/path/a"),
	}}
	snd := &fakeSender{}
	resolve := func(dir string) string { return "owner/a" }

	opt := Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly, DryRun: true}
	rpt, err := Once(context.Background(), q, det, snd, resolve, opt)
	if err != nil {
		t.Fatal(err)
	}

	if len(rpt.Unmatched) != 1 || rpt.Unmatched[0].Repo != "owner/z" {
		t.Errorf("DryRun Unmatched = %v, want [{Repo:owner/z}]", rpt.Unmatched)
	}
}

// ---- Watch: stops on cancel ------------------------------------------------

func TestWatch_StopsOnCancel(t *testing.T) {
	q := newTestQ(t)
	q.Add("hello", "", "") //nolint:errcheck

	det := &fakeDetector{sessions: []session.Session{
		idleSession("s1", "/"),
	}}
	snd := &fakeSender{}

	ctx, cancel := context.WithCancel(context.Background())
	opt := Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly}

	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, q, det, snd, nil, opt, 10*time.Millisecond, nil)
	}()

	// Give it at least one tick.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Watch returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Watch did not stop after context cancel")
	}
}

func TestWatch_EmptySessions_Error(t *testing.T) {
	q := newTestQ(t)
	det := &fakeDetector{}
	snd := &fakeSender{}

	err := Watch(context.Background(), q, det, snd, nil, Options{}, time.Millisecond, nil)
	if err == nil {
		t.Error("Watch with empty Sessions: want error, got nil")
	}
}

// TestWatch_RunsAtTZero verifies that Watch delivers a report immediately at
// t=0, before any ticker fires. It uses a 1-hour ticker with a 200ms timeout:
// if Watch only ran on ticks, no report would arrive before the deadline.
func TestWatch_RunsAtTZero(t *testing.T) {
	q := newTestQ(t)
	q.Add("immediate", "", "")

	det := &fakeDetector{sessions: []session.Session{idleSession("s1", "/")}}
	snd := &fakeSender{}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var reports []Report
	opt := Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly}
	// Ticker every 1 hour → with a 200ms timeout, only the t=0 pass fires.
	Watch(ctx, q, det, snd, nil, opt, time.Hour, func(r Report) { //nolint:errcheck
		reports = append(reports, r)
	})

	if len(reports) == 0 {
		t.Error("Watch must deliver at least one report at t=0, before any tick")
	}
	if len(reports[0].Sent) != 1 {
		t.Errorf("first report Sent = %d, want 1", len(reports[0].Sent))
	}
}

func TestWatch_TransientDetectError_Continues(t *testing.T) {
	q := newTestQ(t)
	q.Add("task", "", "") //nolint:errcheck

	var reports []Report
	callN := 0

	// Context with a generous deadline so Watch fires several ticks.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// First call returns an error; subsequent calls succeed.
	customDet := &callbackDetector{fn: func(ctx context.Context) ([]session.Session, error) {
		callN++
		if callN == 1 {
			return nil, fmt.Errorf("transient detect error")
		}
		return []session.Session{idleSession("s1", "/")}, nil
	}}

	opt := Options{Sessions: []string{"*"}, Mode: session.ModeIdleOnly}
	// Watch blocks until the deadline; tick every 20ms so we get several passes.
	err := Watch(ctx, q, customDet, &fakeSender{}, nil, opt, 20*time.Millisecond, func(r Report) {
		reports = append(reports, r)
	})

	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("Watch returned %v, want a context error", err)
	}
	if len(reports) < 2 {
		t.Errorf("Watch reported %d times, want ≥2 (at least once for error, once for success)", len(reports))
	}
	// First report must carry the detect error in Failed.
	if len(reports[0].Failed) == 0 {
		t.Error("first report (from detect error) should have Failed entries")
	}
}

// TestOnce_Accept_HeldNotSent verifies that a task rejected by Options.Accept
// appears in Held and is never sent, while a later accepted task is sent
// instead (FIFO among accepted).
func TestOnce_Accept_HeldNotSent(t *testing.T) {
	q := newTestQ(t)

	// Add two tasks; the first will be rejected by Accept, the second accepted.
	task1, err := q.Add("unclear task", "", "")
	if err != nil {
		t.Fatal(err)
	}
	task2, err := q.Add("clear task", "", "")
	if err != nil {
		t.Fatal(err)
	}

	det := &fakeDetector{sessions: []session.Session{idleSession("s1", "")}}
	snd := &fakeSender{}

	opt := Options{
		Sessions: []string{"s1"},
		Accept: func(task queue.Task) bool {
			return task.ID != task1.ID // reject task1
		},
	}

	rpt, err := Once(context.Background(), q, det, snd, nil, opt)
	if err != nil {
		t.Fatal(err)
	}

	// task2 should be sent (skipped over task1 which was held).
	if len(rpt.Sent) != 1 || rpt.Sent[0].Task.ID != task2.ID {
		t.Errorf("Sent task IDs = %v, want task %d", sentIDs(rpt.Sent), task2.ID)
	}

	// task1 should be in Held.
	if len(rpt.Held) != 1 || rpt.Held[0].ID != task1.ID {
		t.Errorf("Held task IDs = %v, want task %d", heldIDs(rpt.Held), task1.ID)
	}

	// Sent tasks must NOT appear in Held.
	for _, h := range rpt.Held {
		for _, s := range rpt.Sent {
			if h.ID == s.Task.ID {
				t.Errorf("task %d appears in both Sent and Held", h.ID)
			}
		}
	}
}

// TestOnce_Accept_Nil_AcceptsAll ensures Accept==nil behaves as before (all
// tasks eligible).
func TestOnce_Accept_Nil_AcceptsAll(t *testing.T) {
	q := newTestQ(t)
	task, err := q.Add("any task", "", "")
	if err != nil {
		t.Fatal(err)
	}

	det := &fakeDetector{sessions: []session.Session{idleSession("s1", "")}}
	snd := &fakeSender{}

	rpt, err := Once(context.Background(), q, det, snd, nil, Options{Sessions: []string{"s1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sent) != 1 || rpt.Sent[0].Task.ID != task.ID {
		t.Errorf("Sent = %v, want task %d", sentIDs(rpt.Sent), task.ID)
	}
	if len(rpt.Held) != 0 {
		t.Errorf("Held = %v, want empty when Accept is nil", rpt.Held)
	}
}

// TestOnce_DryRun_Accept_HeldNotSent verifies the same behaviour in DryRun
// mode: rejected tasks appear in Held and are skipped in favour of accepted
// tasks.
func TestOnce_DryRun_Accept_HeldNotSent(t *testing.T) {
	q := newTestQ(t)
	task1, err := q.Add("unclear task", "", "")
	if err != nil {
		t.Fatal(err)
	}
	task2, err := q.Add("clear task", "", "")
	if err != nil {
		t.Fatal(err)
	}

	det := &fakeDetector{sessions: []session.Session{idleSession("s1", "")}}
	snd := &fakeSender{}

	opt := Options{
		Sessions: []string{"s1"},
		DryRun:   true,
		Accept: func(task queue.Task) bool {
			return task.ID != task1.ID
		},
	}

	rpt, err := Once(context.Background(), q, det, snd, nil, opt)
	if err != nil {
		t.Fatal(err)
	}

	// In dry-run, task2 should be in Sent and task1 in Held.
	if len(rpt.Sent) != 1 || rpt.Sent[0].Task.ID != task2.ID {
		t.Errorf("DryRun Sent = %v, want task %d", sentIDs(rpt.Sent), task2.ID)
	}
	if len(rpt.Held) != 1 || rpt.Held[0].ID != task1.ID {
		t.Errorf("DryRun Held = %v, want task %d", heldIDs(rpt.Held), task1.ID)
	}

	// Queue must be untouched in dry-run.
	all, _ := q.List()
	for _, task := range all {
		if task.Status != queue.Pending {
			t.Errorf("dry-run mutated task %d status to %v", task.ID, task.Status)
		}
	}
}

// TestOnce_Accept_UnmatchedNotHeld verifies that repo-mismatched tasks are
// reported as Unmatched, not Held, even when Accept would also reject them.
func TestOnce_Accept_UnmatchedNotHeld(t *testing.T) {
	q := newTestQ(t)
	task1, err := q.Add("task", "owner/other-repo", "")
	if err != nil {
		t.Fatal(err)
	}

	det := &fakeDetector{sessions: []session.Session{idleSession("s1", "/myrepo")}}
	snd := &fakeSender{}

	opt := Options{
		Sessions: []string{"s1"},
		// Accept rejects everything — but task1 is repo-mismatched first.
		Accept: func(task queue.Task) bool { return false },
	}

	rpt, err := Once(context.Background(), q, det, snd, func(string) string { return "owner/my-repo" }, opt)
	if err != nil {
		t.Fatal(err)
	}

	// task1 should be in Unmatched, not Held.
	if len(rpt.Unmatched) != 1 || rpt.Unmatched[0].ID != task1.ID {
		t.Errorf("Unmatched = %v, want task %d", heldIDs(rpt.Unmatched), task1.ID)
	}
	if len(rpt.Held) != 0 {
		t.Errorf("Held = %v, want empty for repo-mismatched task", rpt.Held)
	}
}

func sentIDs(sent []Sent) []int {
	ids := make([]int, len(sent))
	for i, s := range sent {
		ids[i] = s.Task.ID
	}
	return ids
}

func heldIDs(tasks []queue.Task) []int {
	ids := make([]int, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	return ids
}

// callbackDetector is a Detector backed by a function, for custom behaviour.
type callbackDetector struct {
	fn func(ctx context.Context) ([]session.Session, error)
}

func (c *callbackDetector) Detect(ctx context.Context) ([]session.Session, error) {
	return c.fn(ctx)
}
