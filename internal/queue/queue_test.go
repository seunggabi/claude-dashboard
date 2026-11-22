package queue

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newQ(t *testing.T) *Queue {
	t.Helper()
	return Open(filepath.Join(t.TempDir(), "queue.json"))
}

// ---- Add validation --------------------------------------------------------

func TestAdd_EmptyText(t *testing.T) {
	q := newQ(t)
	if _, err := q.Add("", "", ""); err == nil {
		t.Error("Add empty text: want error, got nil")
	}
}

func TestAdd_MultiLineText(t *testing.T) {
	q := newQ(t)
	// \n is a control character and must be rejected.
	if _, err := q.Add("line1\nline2", "", ""); err == nil {
		t.Error("Add multi-line text: want error, got nil")
	}
}

func TestAdd_ControlCharInText(t *testing.T) {
	q := newQ(t)
	if _, err := q.Add("hello\x1bworld", "", ""); err == nil {
		t.Error("Add ESC character: want error, got nil")
	}
}

func TestAdd_BadRepo_NoSlash(t *testing.T) {
	q := newQ(t)
	if _, err := q.Add("hello", "badrepo", ""); err == nil {
		t.Error("Add repo without slash: want error, got nil")
	}
}

func TestAdd_BadRepo_EmptyOwner(t *testing.T) {
	q := newQ(t)
	if _, err := q.Add("hello", "/name", ""); err == nil {
		t.Error("Add repo /name: want error, got nil")
	}
}

func TestAdd_Valid(t *testing.T) {
	q := newQ(t)
	task, err := q.Add("hello world", "owner/repo", "ref-1")
	if err != nil {
		t.Fatalf("Add valid task: %v", err)
	}
	if task.Text != "hello world" {
		t.Errorf("Text = %q, want %q", task.Text, "hello world")
	}
	if task.Repo != "owner/repo" {
		t.Errorf("Repo = %q, want owner/repo", task.Repo)
	}
	if task.Status != Pending {
		t.Errorf("Status = %q, want pending", task.Status)
	}
}

// ---- FIFO Claim ------------------------------------------------------------

func TestClaim_FIFO(t *testing.T) {
	q := newQ(t)
	texts := []string{"first", "second", "third"}
	for _, text := range texts {
		if _, err := q.Add(text, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range texts {
		claimed, err := q.Claim("sess", nil)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if claimed == nil {
			t.Fatal("Claim returned nil, want task")
		}
		if claimed.Text != want {
			t.Errorf("Claim: got %q, want %q", claimed.Text, want)
		}
		if claimed.Status != Dispatched {
			t.Errorf("status = %q, want dispatched", claimed.Status)
		}
		if claimed.Session != "sess" {
			t.Errorf("session = %q, want sess", claimed.Session)
		}
		if claimed.DispatchedAt.IsZero() {
			t.Error("DispatchedAt must not be zero")
		}
	}
	// Queue exhausted.
	extra, err := q.Claim("sess", nil)
	if err != nil {
		t.Fatalf("Claim on empty queue: %v", err)
	}
	if extra != nil {
		t.Errorf("Claim on empty: got %+v, want nil", extra)
	}
}

// ---- Claim with accept filter ----------------------------------------------

func TestClaim_AcceptFilterSkipsNonMatching(t *testing.T) {
	q := newQ(t)
	q.Add("task A", "owner/a", "") //nolint:errcheck
	q.Add("task B", "owner/b", "") //nolint:errcheck
	q.Add("task C", "owner/a", "") //nolint:errcheck

	onlyB := func(task Task) bool { return task.Repo == "owner/b" }
	claimed, err := q.Claim("sess", onlyB)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("Claim: want task B, got nil")
	}
	if claimed.Text != "task B" {
		t.Errorf("Claim: got %q, want task B", claimed.Text)
	}

	// owner/a tasks must still be pending.
	pending, err := q.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Errorf("after claiming B, pending = %d, want 2", len(pending))
	}
}

func TestClaim_AcceptFilterNoMatch(t *testing.T) {
	q := newQ(t)
	q.Add("task A", "owner/a", "") //nolint:errcheck

	never := func(Task) bool { return false }
	claimed, err := q.Claim("sess", never)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed != nil {
		t.Errorf("Claim with always-false filter: got %+v, want nil", claimed)
	}
}

// ---- Release ---------------------------------------------------------------

func TestRelease(t *testing.T) {
	q := newQ(t)
	if _, err := q.Add("hello", "", ""); err != nil {
		t.Fatal(err)
	}

	claimed, err := q.Claim("sess", nil)
	if err != nil || claimed == nil {
		t.Fatalf("Claim: err=%v claimed=%v", err, claimed)
	}

	pending, _ := q.Pending()
	if len(pending) != 0 {
		t.Fatalf("after Claim, pending = %d, want 0", len(pending))
	}

	if err := q.Release(claimed.ID); err != nil {
		t.Fatalf("Release: %v", err)
	}

	pending, _ = q.Pending()
	if len(pending) != 1 {
		t.Fatalf("after Release, pending = %d, want 1", len(pending))
	}
	got := pending[0]
	if got.Status != Pending {
		t.Errorf("released task status = %q, want pending", got.Status)
	}
	if got.Session != "" {
		t.Errorf("released task session = %q, want empty", got.Session)
	}
	if !got.DispatchedAt.IsZero() {
		t.Errorf("released task DispatchedAt should be zero, got %v", got.DispatchedAt)
	}

	// Should be claimable again.
	reclaimed, err := q.Claim("sess2", nil)
	if err != nil || reclaimed == nil {
		t.Fatalf("re-Claim: err=%v reclaimed=%v", err, reclaimed)
	}
	if reclaimed.ID != claimed.ID {
		t.Errorf("re-Claim ID = %d, want %d", reclaimed.ID, claimed.ID)
	}
}

func TestRelease_NonExistentID(t *testing.T) {
	q := newQ(t)
	if err := q.Release(9999); err == nil {
		t.Error("Release non-existent ID: want error, got nil")
	}
}

// ---- Remove ----------------------------------------------------------------

func TestRemove(t *testing.T) {
	q := newQ(t)
	t1, _ := q.Add("one", "", "")
	t2, _ := q.Add("two", "", "")

	if err := q.Remove(t1.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	tasks, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != t2.ID {
		t.Errorf("after Remove, List = %v, want [{id:%d}]", tasks, t2.ID)
	}

	// Removing again must fail.
	if err := q.Remove(t1.ID); err == nil {
		t.Error("Remove already-removed ID: want error, got nil")
	}
}

// ---- IDs never reused ------------------------------------------------------

func TestIDs_NeverReused(t *testing.T) {
	q := newQ(t)
	t1, err := q.Add("first", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Remove(t1.ID); err != nil {
		t.Fatal(err)
	}
	t2, err := q.Add("second", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if t2.ID == t1.ID {
		t.Errorf("ID %d was reused after Remove", t1.ID)
	}
	if t2.ID <= t1.ID {
		t.Errorf("new ID %d not greater than removed ID %d", t2.ID, t1.ID)
	}
}

// ---- ClearDispatched -------------------------------------------------------

func TestClearDispatched(t *testing.T) {
	q := newQ(t)
	q.Add("stay-1", "", "") //nolint:errcheck
	q.Add("stay-2", "", "") //nolint:errcheck
	q.Add("go-1", "", "")   //nolint:errcheck
	q.Add("go-2", "", "")   //nolint:errcheck

	q.Claim("s1", func(task Task) bool { return task.Text == "go-1" }) //nolint:errcheck
	q.Claim("s2", func(task Task) bool { return task.Text == "go-2" }) //nolint:errcheck

	n, err := q.ClearDispatched()
	if err != nil {
		t.Fatalf("ClearDispatched: %v", err)
	}
	if n != 2 {
		t.Errorf("ClearDispatched returned %d, want 2", n)
	}

	tasks, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("after ClearDispatched, List = %d tasks, want 2", len(tasks))
	}
	for _, task := range tasks {
		if task.Status == Dispatched {
			t.Errorf("task %d still dispatched after ClearDispatched", task.ID)
		}
	}
}

func TestClearDispatched_Empty(t *testing.T) {
	q := newQ(t)
	q.Add("only pending", "", "") //nolint:errcheck

	n, err := q.ClearDispatched()
	if err != nil {
		t.Fatalf("ClearDispatched on no dispatched: %v", err)
	}
	if n != 0 {
		t.Errorf("ClearDispatched = %d, want 0", n)
	}
}

// ---- Corrupt JSON ----------------------------------------------------------

func TestCorruptJSON_ClearError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "queue.json")
	corrupt := `{invalid json`
	if err := os.WriteFile(p, []byte(corrupt), 0o644); err != nil {
		t.Fatal(err)
	}

	q := Open(p)
	_, err := q.List()
	if err == nil {
		t.Fatal("corrupt JSON: want error, got nil")
	}
	if !strings.Contains(err.Error(), p) {
		t.Errorf("error %q does not mention file path %q", err.Error(), p)
	}

	// The corrupt file must NOT have been silently overwritten.
	got, readErr := os.ReadFile(p)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != corrupt {
		t.Errorf("corrupt file was overwritten; now contains %q", got)
	}
}

// ---- Concurrent Claim ------------------------------------------------------

func TestConcurrentClaim_NoDuplicates(t *testing.T) {
	const numTasks = 20
	const numWorkers = 50

	q := newQ(t)
	for i := 0; i < numTasks; i++ {
		if _, err := q.Add(fmt.Sprintf("task-%d", i), "", ""); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	timesClaimedByID := make(map[int]int) // task ID → number of goroutines that received it

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			sess := fmt.Sprintf("sess-%d", w)
			task, err := q.Claim(sess, nil)
			if err != nil {
				t.Errorf("worker %d: Claim error: %v", w, err)
				return
			}
			if task == nil {
				return
			}
			mu.Lock()
			timesClaimedByID[task.ID]++
			mu.Unlock()
		}(w)
	}
	wg.Wait()

	for id, count := range timesClaimedByID {
		if count > 1 {
			t.Errorf("task %d was claimed %d times; must be claimed at most once", id, count)
		}
	}
}
