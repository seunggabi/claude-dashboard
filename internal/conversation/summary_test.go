package conversation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLog(t *testing.T, lines ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	workDir := "/work/app"
	dir := mapToProjectDir(workDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "s.jsonl"), []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatal(err)
	}
	return workDir
}

func TestReadSummary_todoWrite(t *testing.T) {
	workDir := writeLog(t,
		`{"type":"user","message":{"role":"user","content":"first prompt"}}`,
		`{"type":"ai-title","aiTitle":"Fix login bug"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"TodoWrite","input":{"todos":[{"content":"old","status":"pending"}]}}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"b","name":"TodoWrite","input":{"todos":[{"content":"read code","status":"completed"},{"content":"write fix","status":"in_progress"}]}}]}}`,
		`{"type":"user","message":{"role":"user","content":"<command-name>/clear</command-name>"}}`,
		`{"type":"user","message":{"role":"user","content":"add a test too"}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"b","content":"ok"}]}}`,
	)

	s, err := ReadSummary(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Fix login bug" || s.LastPrompt != "add a test too" {
		t.Errorf("title=%q prompt=%q", s.Title, s.LastPrompt)
	}
	want := []Todo{{"read code", "completed"}, {"write fix", "in_progress"}}
	if len(s.Todos) != 2 || s.Todos[0] != want[0] || s.Todos[1] != want[1] {
		t.Errorf("todos = %v, want %v", s.Todos, want)
	}
}

func TestReadSummary_tasks(t *testing.T) {
	workDir := writeLog(t,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"c1","name":"TaskCreate","input":{"subject":"one"}},{"type":"tool_use","id":"c2","name":"TaskCreate","input":{"subject":"two"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"c1","content":"ok"}]},"toolUseResult":{"task":{"id":"1","subject":"one"}}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"c2","content":"ok"}]},"toolUseResult":{"task":{"id":2,"subject":"two"}}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"u1","name":"TaskUpdate","input":{"taskId":"1","status":"completed"}},{"type":"tool_use","id":"u2","name":"TaskUpdate","input":{"taskId":"2","status":"deleted"}}]}}`,
	)

	s, err := ReadSummary(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Todos) != 1 || s.Todos[0] != (Todo{"one", "completed"}) {
		t.Errorf("todos = %v, want [{one completed}]", s.Todos)
	}
}
