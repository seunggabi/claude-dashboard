package queue

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Looking at the queue must never create anything: users who never queue a
// task should not get a ~/.claude-dashboard directory from the TUI or a
// dispatcher polling it.
func TestReadingMissingQueueCreatesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-created")
	q := Open(filepath.Join(dir, "queue.json"))

	tasks, err := q.List()
	if err != nil || len(tasks) != 0 {
		t.Fatalf("List on a missing queue = %v, %v; want empty, nil", tasks, err)
	}
	pending, err := q.Pending()
	if err != nil || len(pending) != 0 {
		t.Fatalf("Pending on a missing queue = %v, %v; want empty, nil", pending, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("reading must not create the queue directory (stat err = %v)", err)
	}
}

// List and Pending must not rewrite queue.json: every TUI refresh and every
// dispatch pass reads it.
func TestReadingDoesNotRewriteTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	q := Open(path)
	if _, err := q.Add("update the docs", "", ""); err != nil {
		t.Fatal(err)
	}

	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if _, err := q.List(); err != nil {
			t.Fatal(err)
		}
		if _, err := q.Pending(); err != nil {
			t.Fatal(err)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Fatalf("reading rewrote the file: mtime %v, want %v", info.ModTime(), old)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("reading changed the file contents")
	}
}

// A returned slice is the caller's: changing it must not affect later reads.
func TestListReturnsACopy(t *testing.T) {
	q := Open(filepath.Join(t.TempDir(), "queue.json"))
	if _, err := q.Add("update the docs", "", ""); err != nil {
		t.Fatal(err)
	}
	first, _ := q.List()
	first[0].Text = "tampered"

	second, _ := q.List()
	if second[0].Text != "update the docs" {
		t.Fatalf("List must return an independent copy, got %q", second[0].Text)
	}
}
