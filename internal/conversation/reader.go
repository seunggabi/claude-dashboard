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
func parseJSONL(path string, maxMessages int) ([]Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var messages []Message
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		var entry jsonlEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		if entry.Type != "user" && entry.Type != "assistant" {
			continue
		}
		if entry.Message == nil {
			continue
		}

		content := extractContent(entry.Message)
		if content == "" {
			continue
		}

		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)

		messages = append(messages, Message{
			Role:      entry.Message.Role,
			Content:   content,
			Timestamp: ts,
		})
	}

	// Return last N messages
	if maxMessages > 0 && len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}

	return messages, nil
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
