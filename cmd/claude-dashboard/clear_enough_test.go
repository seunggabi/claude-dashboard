package main

import (
	"testing"

	"github.com/seunggabi/claude-dashboard/internal/queue"
)

func TestClearEnoughHoldsUnclearTextTasks(t *testing.T) {
	// Guidelines must not rescue a vague brief: Brief is what gets scored.
	vague := queue.Task{Brief: "make it better", Text: "make it better Create a new branch, add tests, and open a PR with a clear description."}
	if clearEnough(vague) {
		t.Error("a vague brief must be held even when guidelines make the text long")
	}
}

func TestClearEnoughNeverHoldsIssueTasks(t *testing.T) {
	issue := queue.Task{Ref: "#12 Fix login", Text: "Work on issue #12 in owner/name"}
	if !clearEnough(issue) {
		t.Error("issue tasks carry their requirements in the issue and must not be held")
	}
}

func TestClearEnoughFallsBackToTextWithoutBrief(t *testing.T) {
	if clearEnough(queue.Task{Text: "make it better"}) {
		t.Error("a task with no brief should be scored on its text")
	}
}

// "vague" is borderline, not unclear: only the unclear level is held.
func TestClearEnoughAllowsVagueTasks(t *testing.T) {
	if !clearEnough(queue.Task{Brief: "fix it", Text: "fix it"}) {
		t.Error("a vague (40-69) task must still be dispatched")
	}
}
