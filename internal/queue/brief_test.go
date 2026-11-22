package queue

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddWithBriefKeepsTheUsersWords(t *testing.T) {
	q := Open(filepath.Join(t.TempDir(), "queue.json"))

	task, err := q.AddWithBrief("fix the login bug Create a new branch and open a PR.", "fix the login bug", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if task.Brief != "fix the login bug" || task.Text != "fix the login bug Create a new branch and open a PR." {
		t.Fatalf("unexpected task: %+v", task)
	}

	listed, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Brief != "fix the login bug" {
		t.Fatalf("brief did not survive a round trip: %+v", listed)
	}
}

func TestAddLeavesBriefEmpty(t *testing.T) {
	q := Open(filepath.Join(t.TempDir(), "queue.json"))

	task, err := q.Add("update the docs", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if task.Brief != "" {
		t.Fatalf("Add must not invent a brief, got %q", task.Brief)
	}
}

// Queue files written before Brief existed have no "brief" key.
func TestLoadsQueueFileWithoutBrief(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	old := `{"next_id":2,"tasks":[{"id":1,"text":"old task","added_at":"2026-11-23T10:00:00Z","status":"pending"}]}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks, err := Open(path).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Text != "old task" || tasks[0].Brief != "" {
		t.Fatalf("old queue file did not load cleanly: %+v", tasks)
	}
}
