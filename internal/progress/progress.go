// Package progress extracts per-session to-do list progress from Claude Code
// conversation transcripts (JSONL files under ~/.claude/projects/).
package progress

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Progress holds the current to-do list state for one session.
type Progress struct {
	Done      int
	Total     int
	Current   string    // content of the in_progress item, "" if none; sanitised, single line, ≤60 runes
	UpdatedAt time.Time // time of the last list update; zero if unknown
}

// Percent returns 0–100 (rounded down). Returns 0 when Total == 0.
func (p Progress) Percent() int {
	if p.Total == 0 {
		return 0
	}
	return (p.Done * 100) / p.Total
}

// ForSession returns the progress of the most recent to-do list in the newest
// transcript for the project at dir.
// ok is false when there is no transcript or no list.
// Only the last ~512 KiB of the file is read; a partial first line is tolerated.
func ForSession(dir string) (p Progress, ok bool) {
	projectDir := mapToProjectDir(dir)
	if projectDir == "" {
		return Progress{}, false
	}

	jsonlPath, err := findLatestJSONL(projectDir)
	if err != nil {
		return Progress{}, false
	}

	f, err := os.Open(jsonlPath)
	if err != nil {
		return Progress{}, false
	}
	defer f.Close()

	// Seek to the last tailSize bytes so very large transcripts don't block.
	const tailSize = 512 * 1024
	if info, statErr := f.Stat(); statErr == nil && info.Size() > tailSize {
		if _, seekErr := f.Seek(-tailSize, io.SeekEnd); seekErr != nil {
			_, _ = f.Seek(0, io.SeekStart)
		}
	}

	return Parse(f)
}

// Parse extracts progress from transcript lines (exported for testing without files).
// It scans r line by line; the last valid to-do list update wins.
func Parse(r io.Reader) (Progress, bool) {
	scanner := bufio.NewScanner(r)
	// Support lines up to 10 MiB (tool inputs can be large).
	scanner.Buffer(make([]byte, 0, 128*1024), 10*1024*1024)

	var last Progress
	found := false

	for scanner.Scan() {
		if p, ok := parseLine(scanner.Bytes()); ok {
			last = p
			found = true
		}
	}

	return last, found
}

// ---- internal helpers -------------------------------------------------------

// todoToolNames are the tool call names we recognise as todo-list replacements.
// mcp__ccd_turn__update_plan is the name observed in real transcripts.
// TodoWrite is Claude Code's built-in name (included for forward compatibility).
var todoToolNames = map[string]bool{
	"mcp__ccd_turn__update_plan": true,
	"TodoWrite":                  true,
}

// transcriptLine is the shape of one JSONL entry.
type transcriptLine struct {
	Timestamp string   `json:"timestamp"`
	Message   *lineMsg `json:"message"`
}

type lineMsg struct {
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// todoInput covers both observed variants:
//   - mcp__ccd_turn__update_plan uses input.items
//   - TodoWrite (classic) uses input.todos
type todoInput struct {
	Items []todoItem `json:"items"`
	Todos []todoItem `json:"todos"`
}

// todoItem is a single task.
type todoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`     // pending | in_progress | completed
	ActiveForm string `json:"activeForm"` // older format; may be absent
}

// parseLine attempts to extract a Progress from one JSONL line.
// Returns (zero, false) for lines that carry no todo update.
func parseLine(b []byte) (Progress, bool) {
	var tl transcriptLine
	if err := json.Unmarshal(b, &tl); err != nil {
		return Progress{}, false
	}
	if tl.Message == nil {
		return Progress{}, false
	}

	for i := range tl.Message.Content {
		block := &tl.Message.Content[i]
		if !todoToolNames[block.Name] {
			continue
		}
		if block.Input == nil {
			continue
		}

		var inp todoInput
		if err := json.Unmarshal(block.Input, &inp); err != nil {
			continue
		}

		// Prefer items; fall back to todos.
		items := inp.Items
		if len(items) == 0 {
			items = inp.Todos
		}
		if len(items) == 0 {
			continue
		}

		done := 0
		current := ""
		for _, item := range items {
			switch item.Status {
			case "completed":
				done++
			case "in_progress":
				if current == "" {
					current = sanitise(currentText(item))
				}
			}
		}

		ts, _ := time.Parse(time.RFC3339Nano, tl.Timestamp)
		return Progress{
			Done:      done,
			Total:     len(items),
			Current:   current,
			UpdatedAt: ts,
		}, true
	}

	return Progress{}, false
}

// currentText returns the display text for an in-progress item.
// It prefers activeForm when present (older transcript format).
func currentText(item todoItem) string {
	if item.ActiveForm != "" {
		return item.ActiveForm
	}
	return item.Content
}

// sanitise collapses whitespace to a single space, trims leading/trailing
// whitespace, and truncates to 60 runes.
func sanitise(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	// Truncate to 60 runes.
	if utf8.RuneCountInString(s) > 60 {
		runes := []rune(s)
		s = string(runes[:60])
	}
	return s
}

// ---- path helpers (mirrors unexported helpers in conversation package) -------

// nonAlphanumeric matches characters that Claude Code replaces with "-" when
// encoding a working directory path into a project directory name.
var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)

// mapToProjectDir converts a working directory path to its Claude project
// directory (e.g. /Users/foo/bar → ~/.claude/projects/-Users-foo-bar).
func mapToProjectDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	encoded := nonAlphanumeric.ReplaceAllString(workDir, "-")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".claude", "projects", encoded)
}

// findLatestJSONL returns the path of the most recently modified .jsonl file
// in projectDir.
func findLatestJSONL(projectDir string) (string, error) {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return "", fmt.Errorf("progress: no transcript directory: %w", err)
	}

	type fi struct {
		path    string
		modTime time.Time
	}
	var files []fi

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fi{
			path:    filepath.Join(projectDir, e.Name()),
			modTime: info.ModTime(),
		})
	}

	if len(files) == 0 {
		return "", fmt.Errorf("progress: no .jsonl files in %s", projectDir)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	return files[0].path, nil
}
