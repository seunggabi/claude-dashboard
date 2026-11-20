package conversation

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// Todo is one item of a session's todo list.
type Todo struct {
	Content string
	Status  string // pending, in_progress or completed
}

// Summary is what a session is doing, taken from its conversation log.
type Summary struct {
	Title      string // ai-title written by Claude Code
	LastPrompt string // last prompt typed by the user
	Todos      []Todo
}

// summaryCache keeps the last summary of each log file, so an unchanged log
// is not read again on every refresh.
var (
	summaryMu    sync.Mutex
	summaryCache = map[string]cachedSummary{}
)

type cachedSummary struct {
	modTime time.Time
	size    int64
	summary Summary
}

// ReadSummary reads the most recent conversation log for a working directory
// and returns its title, last prompt and current todo list.
func ReadSummary(workDir string) (Summary, error) {
	jsonlFile, err := findLatestJSONL(mapToProjectDir(workDir))
	if err != nil {
		return Summary{}, err
	}
	info, err := os.Stat(jsonlFile)
	if err != nil {
		return Summary{}, err
	}

	summaryMu.Lock()
	c, ok := summaryCache[jsonlFile]
	summaryMu.Unlock()
	if ok && c.modTime.Equal(info.ModTime()) && c.size == info.Size() {
		return c.summary, nil
	}

	s, err := parseSummary(jsonlFile)
	if err != nil {
		return Summary{}, err
	}
	summaryMu.Lock()
	summaryCache[jsonlFile] = cachedSummary{info.ModTime(), info.Size(), s}
	summaryMu.Unlock()
	return s, nil
}

func parseSummary(jsonlFile string) (Summary, error) {
	f, err := os.Open(jsonlFile)
	if err != nil {
		return Summary{}, err
	}
	defer f.Close()

	var s Summary
	// Newer Claude Code keeps todos with TaskCreate/TaskUpdate instead of
	// TodoWrite. tasks holds them in creation order, taskIndex maps a task id
	// to its index, and pendingCreate maps a TaskCreate tool_use id to its
	// subject until the result carrying the new task id arrives.
	var tasks []Todo
	taskIndex := map[string]int{}
	pendingCreate := map[string]string{}
	usedTasks := false

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var e struct {
			Type    string `json:"type"`
			AITitle string `json:"aiTitle"`
			Message *struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
			// An object for TaskCreate, but a string for failed tools.
			ToolUseResult json.RawMessage `json:"toolUseResult"`
		}
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			continue
		}
		if e.Type == "ai-title" && e.AITitle != "" {
			s.Title = e.AITitle
			continue
		}
		if e.Message == nil {
			continue
		}

		// A typed prompt is a plain string. Strings starting with "<" are
		// slash command and system wrappers, not prompts.
		var text string
		if e.Type == "user" && json.Unmarshal(e.Message.Content, &text) == nil {
			if text = strings.TrimSpace(text); text != "" && !strings.HasPrefix(text, "<") {
				s.LastPrompt = text
			}
			continue
		}

		var blocks []struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			ToolUseID string          `json:"tool_use_id"`
			Input     json.RawMessage `json:"input"`
		}
		if json.Unmarshal(e.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			switch {
			case b.Type == "tool_use" && b.Name == "TodoWrite":
				var in struct {
					Todos []struct {
						Content string `json:"content"`
						Status  string `json:"status"`
					} `json:"todos"`
				}
				if json.Unmarshal(b.Input, &in) == nil {
					s.Todos = s.Todos[:0:0]
					for _, t := range in.Todos {
						s.Todos = append(s.Todos, Todo{t.Content, t.Status})
					}
				}
			case b.Type == "tool_use" && b.Name == "TaskCreate":
				var in struct {
					Subject string `json:"subject"`
				}
				if json.Unmarshal(b.Input, &in) == nil {
					pendingCreate[b.ID] = in.Subject
				}
			case b.Type == "tool_use" && b.Name == "TaskUpdate":
				var in struct {
					TaskID string `json:"taskId"`
					Status string `json:"status"`
				}
				if json.Unmarshal(b.Input, &in) == nil && in.Status != "" {
					if i, ok := taskIndex[in.TaskID]; ok {
						tasks[i].Status = in.Status
					}
				}
			case b.Type == "tool_result":
				// TaskCreate's new id arrives with its result.
				subject, ok := pendingCreate[b.ToolUseID]
				if id := createdTaskID(e.ToolUseResult); ok && id != "" {
					delete(pendingCreate, b.ToolUseID)
					taskIndex[id] = len(tasks)
					tasks = append(tasks, Todo{subject, "pending"})
					usedTasks = true
				}
			}
		}
	}

	if usedTasks {
		s.Todos = nil
		for _, t := range tasks {
			if t.Status != "deleted" {
				s.Todos = append(s.Todos, t)
			}
		}
	}
	// A scanner error (such as a line over the buffer limit) stops the read
	// early. What was parsed so far is still worth showing.
	return s, nil
}

// createdTaskID returns the task id from a TaskCreate result, accepting a
// string or a number, or "" when raw is not a TaskCreate result.
func createdTaskID(raw json.RawMessage) string {
	var r struct {
		Task struct {
			ID json.RawMessage `json:"id"`
		} `json:"task"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return ""
	}
	return strings.Trim(string(r.Task.ID), `"`)
}
