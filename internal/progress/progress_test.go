package progress

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helper: build a single assistant JSONL line with a mcp__ccd_turn__update_plan call.
func makeLine(ts string, items []map[string]string) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, fmt.Sprintf(`{"content":%q,"status":%q}`, it["content"], it["status"]))
	}
	itemsJSON := "[" + strings.Join(parts, ",") + "]"
	return fmt.Sprintf(
		`{"timestamp":%q,"message":{"role":"assistant","content":[{"type":"tool_use","name":"mcp__ccd_turn__update_plan","input":{"items":%s,"scope":"session"}}]}}`,
		ts, itemsJSON,
	)
}

// helper: build a TodoWrite JSONL line (classic built-in format, uses "todos" key).
func makeTodoWriteLine(ts string, items []map[string]string) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, fmt.Sprintf(`{"content":%q,"status":%q}`, it["content"], it["status"]))
	}
	todosJSON := "[" + strings.Join(parts, ",") + "]"
	return fmt.Sprintf(
		`{"timestamp":%q,"message":{"role":"assistant","content":[{"type":"tool_use","name":"TodoWrite","input":{"todos":%s}}]}}`,
		ts, todosJSON,
	)
}

// TestSingleList verifies basic Done/Total/Current extraction.
func TestSingleList(t *testing.T) {
	line := makeLine("2026-01-01T10:00:00Z", []map[string]string{
		{"content": "step one", "status": "completed"},
		{"content": "step two", "status": "in_progress"},
		{"content": "step three", "status": "pending"},
	})

	p, ok := Parse(strings.NewReader(line))
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Done != 1 {
		t.Errorf("Done: got %d, want 1", p.Done)
	}
	if p.Total != 3 {
		t.Errorf("Total: got %d, want 3", p.Total)
	}
	if p.Current != "step two" {
		t.Errorf("Current: got %q, want %q", p.Current, "step two")
	}
	if p.Percent() != 33 {
		t.Errorf("Percent: got %d, want 33", p.Percent())
	}
	if p.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

// TestLaterListReplaces verifies that the last TodoWrite entry wins.
func TestLaterListReplaces(t *testing.T) {
	early := makeLine("2026-01-01T09:00:00Z", []map[string]string{
		{"content": "old task", "status": "pending"},
	})
	late := makeLine("2026-01-01T10:00:00Z", []map[string]string{
		{"content": "new task A", "status": "completed"},
		{"content": "new task B", "status": "pending"},
	})
	input := early + "\n" + late

	p, ok := Parse(strings.NewReader(input))
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Total != 2 {
		t.Errorf("Total: got %d, want 2 (later list should replace earlier)", p.Total)
	}
	if p.Done != 1 {
		t.Errorf("Done: got %d, want 1", p.Done)
	}
}

// TestInProgressBecomesCurrente verifies in_progress item becomes Current.
func TestInProgressBecomesCurrent(t *testing.T) {
	line := makeLine("2026-01-01T10:00:00Z", []map[string]string{
		{"content": "first", "status": "pending"},
		{"content": "active work", "status": "in_progress"},
		{"content": "last", "status": "pending"},
	})

	p, ok := Parse(strings.NewReader(line))
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Current != "active work" {
		t.Errorf("Current: got %q, want %q", p.Current, "active work")
	}
}

// TestAllCompleted100Percent verifies Percent returns 100 when all items are done.
func TestAllCompleted100Percent(t *testing.T) {
	line := makeLine("2026-01-01T10:00:00Z", []map[string]string{
		{"content": "a", "status": "completed"},
		{"content": "b", "status": "completed"},
		{"content": "c", "status": "completed"},
	})

	p, ok := Parse(strings.NewReader(line))
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Percent() != 100 {
		t.Errorf("Percent: got %d, want 100", p.Percent())
	}
	if p.Current != "" {
		t.Errorf("Current: got %q, want empty (all done)", p.Current)
	}
}

// TestEmptyListIgnored verifies that an items:[] entry is skipped.
func TestEmptyListIgnored(t *testing.T) {
	emptyLine := `{"timestamp":"2026-01-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"mcp__ccd_turn__update_plan","input":{"items":[],"scope":"session"}}]}}`
	realLine := makeLine("2026-01-01T11:00:00Z", []map[string]string{
		{"content": "real task", "status": "pending"},
	})

	// empty alone → no result
	_, ok := Parse(strings.NewReader(emptyLine))
	if ok {
		t.Error("empty list should not produce ok=true")
	}

	// empty followed by real → real wins
	p, ok := Parse(strings.NewReader(emptyLine + "\n" + realLine))
	if !ok {
		t.Fatal("expected ok=true when real list follows empty")
	}
	if p.Total != 1 {
		t.Errorf("Total: got %d, want 1", p.Total)
	}
}

// TestMalformedAndTruncated verifies that malformed and partial lines are tolerated.
func TestMalformedAndTruncated(t *testing.T) {
	truncated := `{"timestamp":"2026-01-01T10:00:00Z","message":{"role` // cut off
	malformed := `not json at all`
	valid := makeLine("2026-01-01T12:00:00Z", []map[string]string{
		{"content": "good", "status": "completed"},
	})

	input := truncated + "\n" + malformed + "\n" + valid

	p, ok := Parse(strings.NewReader(input))
	if !ok {
		t.Fatal("expected ok=true when at least one valid line exists")
	}
	if p.Total != 1 || p.Done != 1 {
		t.Errorf("unexpected progress: %+v", p)
	}
}

// setupHome creates a temporary HOME and returns the project directory path for
// workDir so the caller can place .jsonl fixtures there.
func setupHome(t *testing.T, workDir string) (projectDir string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	encoded := nonAlphanumeric.ReplaceAllString(workDir, "-")
	projectDir = filepath.Join(home, ".claude", "projects", encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return projectDir
}

// writeJSONL writes lines to a new .jsonl file in projectDir and returns its path.
func writeJSONL(t *testing.T, projectDir, name string, lines []string) string {
	t.Helper()
	path := filepath.Join(projectDir, name)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestForSessionNewestFileWins verifies that among several .jsonl files the
// most recently modified one is used.
func TestForSessionNewestFileWins(t *testing.T) {
	workDir := "/fake/newest/wins"
	projectDir := setupHome(t, workDir)

	// old.jsonl has Done=0/Total=1; new.jsonl has Done=1/Total=1.
	oldPath := writeJSONL(t, projectDir, "old.jsonl", []string{
		makeLine("2026-01-01T08:00:00Z", []map[string]string{
			{"content": "stale", "status": "pending"},
		}),
	})
	newPath := writeJSONL(t, projectDir, "new.jsonl", []string{
		makeLine("2026-01-01T12:00:00Z", []map[string]string{
			{"content": "done", "status": "completed"},
		}),
	})

	// Force old.jsonl to be older.
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldPath, past, past); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(newPath, now, now); err != nil {
		t.Fatal(err)
	}

	p, ok := ForSession(workDir)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Done != 1 || p.Total != 1 {
		t.Errorf("wanted Done=1/Total=1 from newest file; got Done=%d Total=%d", p.Done, p.Total)
	}
}

// TestForSessionTailOnly verifies that ForSession reads only the last 512 KiB.
// The file is >3× tailSize; the first section has a 1-item list; the tail has
// a 5-item list. ForSession must return the 5-item result.
func TestForSessionTailOnly(t *testing.T) {
	workDir := "/fake/tail/only"
	projectDir := setupHome(t, workDir)

	earlyLine := makeLine("2026-01-01T01:00:00Z", []map[string]string{
		{"content": "buried", "status": "pending"},
	})
	padLine := `{"type":"pad","timestamp":"2026-01-01T02:00:00Z"}` + "\n"

	const tailSize = 512 * 1024
	var buf bytes.Buffer
	buf.WriteString(earlyLine + "\n")
	// Pad to 3× tailSize so the early line is well outside the read window.
	for buf.Len() < 3*tailSize {
		buf.WriteString(padLine)
	}
	lateItems := []map[string]string{
		{"content": "t1", "status": "completed"},
		{"content": "t2", "status": "completed"},
		{"content": "t3", "status": "in_progress"},
		{"content": "t4", "status": "pending"},
		{"content": "t5", "status": "pending"},
	}
	buf.WriteString(makeLine("2026-01-01T23:00:00Z", lateItems) + "\n")

	path := filepath.Join(projectDir, "session.jsonl")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	p, ok := ForSession(workDir)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Total != 5 {
		t.Errorf("Total: got %d, want 5 (result must come from the tail)", p.Total)
	}
	if p.Done != 2 {
		t.Errorf("Done: got %d, want 2", p.Done)
	}
	if p.Current != "t3" {
		t.Errorf("Current: got %q, want %q", p.Current, "t3")
	}
}

// TestForSessionPathEncoding verifies that a workDir containing dots, spaces,
// and underscores maps to the expected project directory (all non-alphanumeric
// chars replaced by "-").
func TestForSessionPathEncoding(t *testing.T) {
	// All non-alphanumeric chars → "-"
	// "/my.project/path with_spaces" → "-my-project-path-with-spaces"
	workDir := "/my.project/path with_spaces"
	wantEncoded := "-my-project-path-with-spaces"

	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := filepath.Join(home, ".claude", "projects", wantEncoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, projectDir, "sess.jsonl", []string{
		makeLine("2026-01-01T10:00:00Z", []map[string]string{
			{"content": "encoded", "status": "completed"},
		}),
	})

	p, ok := ForSession(workDir)
	if !ok {
		t.Fatal("expected ok=true; path encoding must map workDir to the right directory")
	}
	if p.Total != 1 || p.Done != 1 {
		t.Errorf("unexpected progress %+v", p)
	}
}

// TestForSessionNoClaudioDir verifies ok=false when ~/.claude does not exist.
func TestForSessionNoClaudioDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // fresh empty home, no .claude subdir
	_, ok := ForSession("/any/work/dir")
	if ok {
		t.Error("expected ok=false when ~/.claude/projects does not exist")
	}
}

// TestForSessionEmptyListOnly verifies ok=false when the transcript's only list
// is empty (items:[]).
func TestForSessionEmptyListOnly(t *testing.T) {
	workDir := "/fake/empty/list"
	projectDir := setupHome(t, workDir)

	emptyLine := `{"timestamp":"2026-01-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"mcp__ccd_turn__update_plan","input":{"items":[],"scope":"session"}}]}}`
	writeJSONL(t, projectDir, "sess.jsonl", []string{emptyLine})

	_, ok := ForSession(workDir)
	if ok {
		t.Error("expected ok=false when the only list is empty")
	}
}

// TestPercentZeroWhenTotalZero verifies Percent() returns 0 for zero Total.
func TestPercentZeroWhenTotalZero(t *testing.T) {
	p := Progress{}
	if p.Percent() != 0 {
		t.Errorf("Percent: got %d, want 0 for empty Progress", p.Percent())
	}
}

// TestCurrentTruncatedAt60Runes verifies that long in_progress content is capped.
func TestCurrentTruncatedAt60Runes(t *testing.T) {
	long := strings.Repeat("가", 80) // 80 Korean runes, each multi-byte
	line := makeLine("2026-01-01T10:00:00Z", []map[string]string{
		{"content": long, "status": "in_progress"},
	})

	p, ok := Parse(strings.NewReader(line))
	if !ok {
		t.Fatal("expected ok=true")
	}
	runes := []rune(p.Current)
	if len(runes) > 60 {
		t.Errorf("Current is %d runes, want ≤60", len(runes))
	}
}

// TestTodoWriteFormat verifies the classic TodoWrite tool name (todos key) is supported.
func TestTodoWriteFormat(t *testing.T) {
	line := makeTodoWriteLine("2026-01-01T10:00:00Z", []map[string]string{
		{"content": "task A", "status": "completed"},
		{"content": "task B", "status": "in_progress"},
	})

	p, ok := Parse(strings.NewReader(line))
	if !ok {
		t.Fatal("expected ok=true for TodoWrite format")
	}
	if p.Total != 2 {
		t.Errorf("Total: got %d, want 2", p.Total)
	}
	if p.Done != 1 {
		t.Errorf("Done: got %d, want 1", p.Done)
	}
	if p.Current != "task B" {
		t.Errorf("Current: got %q, want %q", p.Current, "task B")
	}
}

// TestUpdateAtTimestamp verifies UpdatedAt is set from the line timestamp.
func TestUpdateAtTimestamp(t *testing.T) {
	wantTS := "2026-06-15T08:30:00Z"
	line := makeLine(wantTS, []map[string]string{
		{"content": "x", "status": "pending"},
	})

	p, ok := Parse(strings.NewReader(line))
	if !ok {
		t.Fatal("expected ok=true")
	}
	want, _ := time.Parse(time.RFC3339, wantTS)
	if !p.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt: got %v, want %v", p.UpdatedAt, want)
	}
}
