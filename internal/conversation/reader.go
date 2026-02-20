package conversation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Message represents a parsed conversation message.
type Message struct {
	Role      string // "user" or "assistant"
	Content   string
	Timestamp time.Time
}

// ReadConversation reads the most recent conversation log for a given working directory.
func ReadConversation(workDir string, maxMessages int) ([]Message, error) {
	projectDir := mapToProjectDir(workDir)
	if projectDir == "" {
		return nil, fmt.Errorf("could not map working directory")
	}

	jsonlFile, err := findLatestJSONL(projectDir)
	if err != nil {
		return nil, err
	}

	return parseJSONL(jsonlFile, maxMessages)
}

// mapToProjectDir converts a working directory to the Claude project directory path.
func mapToProjectDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	// /Users/foo/bar -> -Users-foo-bar
	projectName := strings.ReplaceAll(workDir, "/", "-")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".claude", "projects", projectName)
}

// findLatestJSONL finds the most recently modified .jsonl file in the project directory.
func findLatestJSONL(projectDir string) (string, error) {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return "", fmt.Errorf("no conversation logs found")
	}

	type fileInfo struct {
		path    string
		modTime time.Time
	}
	var jsonlFiles []fileInfo

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		jsonlFiles = append(jsonlFiles, fileInfo{
			path:    filepath.Join(projectDir, entry.Name()),
			modTime: info.ModTime(),
		})
	}

	if len(jsonlFiles) == 0 {
		return "", fmt.Errorf("no .jsonl files found")
	}

	sort.Slice(jsonlFiles, func(i, j int) bool {
		return jsonlFiles[i].modTime.After(jsonlFiles[j].modTime)
	})

	return jsonlFiles[0].path, nil
}

// jsonlEntry represents a raw .jsonl line.
type jsonlEntry struct {
	Type      string    `json:"type"`
	Message   *msgEntry `json:"message,omitempty"`
	Timestamp string    `json:"timestamp"`
}

type msgEntry struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// parseJSONL reads a .jsonl file and extracts conversation messages.
// When maxMessages > 0 it uses a ring buffer so only the last N messages
// are kept in memory instead of reading everything then slicing.
func parseJSONL(path string, maxMessages int) ([]Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	if maxMessages <= 0 {
		// No limit: collect all messages.
		var messages []Message
		for scanner.Scan() {
			if msg, ok := scanLine(scanner.Bytes()); ok {
				messages = append(messages, msg)
			}
		}
		return messages, nil
	}

	// Ring buffer: keep only the last maxMessages entries.
	ring := make([]Message, maxMessages)
	head := 0  // next write position
	count := 0 // total messages seen

	for scanner.Scan() {
		msg, ok := scanLine(scanner.Bytes())
		if !ok {
			continue
		}
		ring[head] = msg
		head = (head + 1) % maxMessages
		count++
	}

	if count == 0 {
		return nil, nil
	}

	// Reconstruct ordered slice from ring buffer.
	size := count
	if size > maxMessages {
		size = maxMessages
	}
	result := make([]Message, size)
	start := (head - size + maxMessages*2) % maxMessages // wrap-safe start
	for i := 0; i < size; i++ {
		result[i] = ring[(start+i)%maxMessages]
	}
	return result, nil
}

// scanLine parses a single JSONL scanner line and returns the Message and true
// if it represents a user or assistant message with non-empty content.
func scanLine(b []byte) (Message, bool) {
	var entry jsonlEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		return Message{}, false
	}
	if entry.Type != "user" && entry.Type != "assistant" {
		return Message{}, false
	}
	if entry.Message == nil {
		return Message{}, false
	}
	content := extractContent(entry.Message)
	if content == "" {
		return Message{}, false
	}
	ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
	return Message{
		Role:      entry.Message.Role,
		Content:   content,
		Timestamp: ts,
	}, true
}

// extractContent extracts text content from a message.
func extractContent(msg *msgEntry) string {
	if msg.Content == nil {
		return ""
	}

	// User messages: content is a string
	if str, ok := msg.Content.(string); ok {
		return str
	}

	// Assistant messages: content is an array of content blocks
	blocks, ok := msg.Content.([]interface{})
	if !ok {
		return ""
	}

	var texts []string
	for _, block := range blocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		blockType, _ := blockMap["type"].(string)
		if blockType == "text" {
			if text, ok := blockMap["text"].(string); ok && text != "" {
				texts = append(texts, text)
			}
		}
	}

	return strings.Join(texts, "\n")
}

// FormatConversation formats messages for display in the log viewer.
func FormatConversation(messages []Message) string {
	var b strings.Builder
	for _, msg := range messages {
		ts := msg.Timestamp.Format("15:04:05")
		switch msg.Role {
		case "user":
			b.WriteString(fmt.Sprintf("─── User [%s] ───\n", ts))
		case "assistant":
			b.WriteString(fmt.Sprintf("─── Assistant [%s] ───\n", ts))
		}
		b.WriteString(msg.Content)
		b.WriteString("\n\n")
	}
	return b.String()
}
