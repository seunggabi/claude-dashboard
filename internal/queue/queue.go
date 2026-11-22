// Package queue is a small file-backed task queue shared by every
// claude-dashboard process on the machine (the CLI, a running dispatcher, the
// TUI).
package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/seunggabi/claude-dashboard/internal/config"
	"github.com/seunggabi/claude-dashboard/internal/tmux"
)

// Status is where a task is in its life.
type Status string

const (
	// Pending tasks wait for a free session.
	Pending Status = "pending"
	// Dispatched tasks were typed into a session.
	Dispatched Status = "dispatched"
)

// repoRe matches "owner/name" as GitHub spells it.
var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Task is one unit of work: a single line of text typed into a Claude session.
type Task struct {
	ID int `json:"id"`
	// Text is what gets typed into the session.
	Text string `json:"text"`
	// Repo, when set ("owner/name"), restricts the task to sessions working in
	// a clone of that repository. Empty means any session.
	Repo string `json:"repo,omitempty"`
	// Ref is a short label for humans, e.g. "#123 Fix login".
	Ref string `json:"ref,omitempty"`
	// Brief is what the user wrote, before shared guidelines were appended to
	// Text. Scoring and listings use it so the guidelines do not skew them.
	Brief        string    `json:"brief,omitempty"`
	AddedAt      time.Time `json:"added_at"`
	Status       Status    `json:"status"`
	Session      string    `json:"session,omitempty"`
	DispatchedAt time.Time `json:"dispatched_at,omitzero"`
}

type state struct {
	NextID int    `json:"next_id"`
	Tasks  []Task `json:"tasks"`
}

// Queue is a task queue stored in one JSON file. Every operation takes an
// exclusive lock on a sibling lock file, so a CLI call, a running dispatcher
// and the TUI never overwrite each other.
type Queue struct {
	path string
}

// DefaultPath is where the queue lives: ~/.claude-dashboard/queue.json.
func DefaultPath() string {
	return filepath.Join(config.ConfigDir(), "queue.json")
}

// Open returns a queue backed by the file at path. The file is created on the
// first write.
func Open(path string) *Queue {
	return &Queue{path: path}
}

// update loads the state under an exclusive lock, lets fn change it, and
// writes it back atomically when fn succeeds.
func (q *Queue) update(fn func(*state) error) error {
	if err := os.MkdirAll(filepath.Dir(q.path), 0o755); err != nil {
		return fmt.Errorf("failed to create queue directory: %w", err)
	}
	lock, err := os.OpenFile(q.path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open queue lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()

	st, err := q.load()
	if err != nil {
		return err
	}
	if err := fn(st); err != nil {
		return err
	}
	return q.save(st)
}

func (q *Queue) load() (*state, error) {
	data, err := os.ReadFile(q.path)
	if errors.Is(err, os.ErrNotExist) {
		return &state{NextID: 1}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read queue: %w", err)
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("queue file %s is corrupt (fix or remove it): %w", q.path, err)
	}
	if st.NextID < 1 {
		st.NextID = 1
	}
	return &st, nil
}

func (q *Queue) save(st *state) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := q.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("failed to write queue: %w", err)
	}
	if err := os.Rename(tmp, q.path); err != nil {
		return fmt.Errorf("failed to replace queue: %w", err)
	}
	return nil
}

// Add appends a pending task. The text must be typeable into a session
// (a single line, see tmux.ValidateSendText), so a bad task is rejected now
// rather than when a session frees up. repo and ref are optional.
func (q *Queue) Add(text, repo, ref string) (Task, error) {
	return q.AddWithBrief(text, "", repo, ref)
}

// AddWithBrief is Add for a task whose text is the user's brief plus extra
// text (such as shared guidelines): brief keeps the user's own words.
func (q *Queue) AddWithBrief(text, brief, repo, ref string) (Task, error) {
	if err := tmux.ValidateSendText(text); err != nil {
		return Task{}, err
	}
	if repo != "" && !repoRe.MatchString(repo) {
		return Task{}, fmt.Errorf("invalid repo %q: expected owner/name", repo)
	}
	var added Task
	err := q.update(func(st *state) error {
		added = Task{ID: st.NextID, Text: text, Brief: brief, Repo: repo, Ref: ref, AddedAt: time.Now().UTC(), Status: Pending}
		st.NextID++
		st.Tasks = append(st.Tasks, added)
		return nil
	})
	return added, err
}

// List returns every task, oldest first.
func (q *Queue) List() ([]Task, error) {
	var tasks []Task
	err := q.update(func(st *state) error {
		tasks = append(tasks, st.Tasks...)
		return nil
	})
	return tasks, err
}

// Pending returns the tasks still waiting, oldest first.
func (q *Queue) Pending() ([]Task, error) {
	all, err := q.List()
	if err != nil {
		return nil, err
	}
	var out []Task
	for _, t := range all {
		if t.Status == Pending {
			out = append(out, t)
		}
	}
	return out, nil
}

// Claim marks the oldest pending task that accept approves as dispatched to
// session and returns it; a nil accept approves every task. It returns nil
// when nothing fits. Claiming before typing keeps two dispatchers from sending
// the same task; call Release if typing fails.
func (q *Queue) Claim(session string, accept func(Task) bool) (*Task, error) {
	var claimed *Task
	err := q.update(func(st *state) error {
		for i := range st.Tasks {
			t := st.Tasks[i]
			if t.Status != Pending || (accept != nil && !accept(t)) {
				continue
			}
			st.Tasks[i].Status = Dispatched
			st.Tasks[i].Session = session
			st.Tasks[i].DispatchedAt = time.Now().UTC()
			c := st.Tasks[i]
			claimed = &c
			return nil
		}
		return nil
	})
	return claimed, err
}

// Release puts a claimed task back to pending, for when typing it failed.
func (q *Queue) Release(id int) error {
	return q.update(func(st *state) error {
		for i := range st.Tasks {
			if st.Tasks[i].ID == id {
				st.Tasks[i].Status = Pending
				st.Tasks[i].Session = ""
				st.Tasks[i].DispatchedAt = time.Time{}
				return nil
			}
		}
		return fmt.Errorf("no task with id %d", id)
	})
}

// Remove deletes a task by id.
func (q *Queue) Remove(id int) error {
	return q.update(func(st *state) error {
		for i := range st.Tasks {
			if st.Tasks[i].ID == id {
				st.Tasks = append(st.Tasks[:i], st.Tasks[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("no task with id %d", id)
	})
}

// ClearDispatched removes every dispatched task and returns how many.
func (q *Queue) ClearDispatched() (int, error) {
	removed := 0
	err := q.update(func(st *state) error {
		kept := st.Tasks[:0]
		for _, t := range st.Tasks {
			if t.Status == Dispatched {
				removed++
				continue
			}
			kept = append(kept, t)
		}
		st.Tasks = kept
		return nil
	})
	return removed, err
}
