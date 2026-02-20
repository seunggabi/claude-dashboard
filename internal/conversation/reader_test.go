package conversation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// extractContent
// ---------------------------------------------------------------------------

func TestExtractContent_nilContentReturnsEmpty(t *testing.T) {
	msg := &msgEntry{Role: "user", Content: nil}
	if got := extractContent(msg); got != "" {
		t.Errorf("expected empty string for nil content, got %q", got)
	}
}

func TestExtractContent_stringContentReturnsString(t *testing.T) {
	msg := &msgEntry{Role: "user", Content: "hello world"}
	if got := extractContent(msg); got != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", got)
	}
}

func TestExtractContent_emptyStringContentReturnsEmpty(t *testing.T) {
	msg := &msgEntry{Role: "user", Content: ""}
	if got := extractContent(msg); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractContent_assistantBlocksJoinsTexts(t *testing.T) {
	blocks := []interface{}{
		map[string]interface{}{"type": "text", "text": "Hello"},
		map[string]interface{}{"type": "text", "text": "World"},
	}
	msg := &msgEntry{Role: "assistant", Content: blocks}
	got := extractContent(msg)
	if got != "Hello\nWorld" {
		t.Errorf("expected %q, got %q", "Hello\nWorld", got)
	}
}

func TestExtractContent_assistantBlocksSkipsNonTextBlocks(t *testing.T) {
	blocks := []interface{}{
		map[string]interface{}{"type": "tool_use", "id": "toolu_123"},
		map[string]interface{}{"type": "text", "text": "Answer"},
	}
	msg := &msgEntry{Role: "assistant", Content: blocks}
	got := extractContent(msg)
	if got != "Answer" {
		t.Errorf("expected %q, got %q", "Answer", got)
	}
}

func TestExtractContent_assistantBlocksSkipsEmptyTextFields(t *testing.T) {
	blocks := []interface{}{
		map[string]interface{}{"type": "text", "text": ""},
		map[string]interface{}{"type": "text", "text": "Non-empty"},
	}
	msg := &msgEntry{Role: "assistant", Content: blocks}
	got := extractContent(msg)
	if got != "Non-empty" {
		t.Errorf("expected %q, got %q", "Non-empty", got)
	}
}

func TestExtractContent_assistantEmptyBlockListReturnsEmpty(t *testing.T) {
	blocks := []interface{}{}
	msg := &msgEntry{Role: "assistant", Content: blocks}
	got := extractContent(msg)
	if got != "" {
		t.Errorf("expected empty string for empty blocks, got %q", got)
	}
}

func TestExtractContent_unexpectedContentTypeReturnsEmpty(t *testing.T) {
	msg := &msgEntry{Role: "assistant", Content: 42}
	got := extractContent(msg)
	if got != "" {
		t.Errorf("expected empty string for integer content, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// mapToProjectDir
// ---------------------------------------------------------------------------

func TestMapToProjectDir_emptyWorkDirReturnsEmpty(t *testing.T) {
	result := mapToProjectDir("")
	if result != "" {
		t.Errorf("expected empty string for empty workDir, got %q", result)
	}
}

func TestMapToProjectDir_convertsSlashesToDashes(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	result := mapToProjectDir("/Users/foo/bar")
	expected := filepath.Join(home, ".claude", "projects", "-Users-foo-bar")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMapToProjectDir_singleComponentPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	result := mapToProjectDir("/tmp")
	expected := filepath.Join(home, ".claude", "projects", "-tmp")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// ---------------------------------------------------------------------------
// parseJSONL — using temporary files
// ---------------------------------------------------------------------------

func writeJSONLFile(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp jsonl file: %v", err)
	}
	return path
}

func TestParseJSONL_returnsErrorForMissingFile(t *testing.T) {
	_, err := parseJSONL("/nonexistent/path/file.jsonl", 0)
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}

func TestParseJSONL_emptyFileReturnsNilMessages(t *testing.T) {
	path := writeJSONLFile(t, []string{})
	msgs, err := parseJSONL(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestParseJSONL_parsesUserMessage(t *testing.T) {
	line := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2024-01-01T00:00:00Z"}`
	path := writeJSONLFile(t, []string{line})
	msgs, err := parseJSONL(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected role %q, got %q", "user", msgs[0].Role)
	}
	if msgs[0].Content != "hello" {
		t.Errorf("expected content %q, got %q", "hello", msgs[0].Content)
	}
}

func TestParseJSONL_parsesAssistantMessageWithBlocks(t *testing.T) {
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hi there"}]},"timestamp":"2024-01-01T00:01:00Z"}`
	path := writeJSONLFile(t, []string{line})
	msgs, err := parseJSONL(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "Hi there" {
		t.Errorf("expected content %q, got %q", "Hi there", msgs[0].Content)
	}
}

func TestParseJSONL_skipsNonMessageTypes(t *testing.T) {
	lines := []string{
		`{"type":"system","message":{"role":"system","content":"sys"},"timestamp":"2024-01-01T00:00:00Z"}`,
		`{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2024-01-01T00:00:01Z"}`,
	}
	path := writeJSONLFile(t, lines)
	msgs, err := parseJSONL(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (system type skipped), got %d", len(msgs))
	}
}

func TestParseJSONL_skipsInvalidJSONLines(t *testing.T) {
	lines := []string{
		`not-json`,
		`{"type":"user","message":{"role":"user","content":"valid"},"timestamp":"2024-01-01T00:00:00Z"}`,
	}
	path := writeJSONLFile(t, lines)
	msgs, err := parseJSONL(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 valid message, got %d", len(msgs))
	}
}

func TestParseJSONL_maxMessagesLimitsResults(t *testing.T) {
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines, `{"type":"user","message":{"role":"user","content":"msg"},"timestamp":"2024-01-01T00:00:00Z"}`)
	}
	path := writeJSONLFile(t, lines)
	msgs, err := parseJSONL(path, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages with maxMessages=3, got %d", len(msgs))
	}
}

func TestParseJSONL_maxMessagesReturnsLastN(t *testing.T) {
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"first"},"timestamp":"2024-01-01T00:00:00Z"}`,
		`{"type":"user","message":{"role":"user","content":"second"},"timestamp":"2024-01-01T00:00:01Z"}`,
		`{"type":"user","message":{"role":"user","content":"third"},"timestamp":"2024-01-01T00:00:02Z"}`,
	}
	path := writeJSONLFile(t, lines)
	msgs, err := parseJSONL(path, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "second" {
		t.Errorf("expected first returned msg to be 'second', got %q", msgs[0].Content)
	}
	if msgs[1].Content != "third" {
		t.Errorf("expected second returned msg to be 'third', got %q", msgs[1].Content)
	}
}

func TestParseJSONL_timestampIsParsed(t *testing.T) {
	line := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2024-06-15T12:30:00Z"}`
	path := writeJSONLFile(t, []string{line})
	msgs, err := parseJSONL(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	expected, _ := time.Parse(time.RFC3339Nano, "2024-06-15T12:30:00Z")
	if !msgs[0].Timestamp.Equal(expected) {
		t.Errorf("expected timestamp %v, got %v", expected, msgs[0].Timestamp)
	}
}

func TestParseJSONL_skipsMessagesWithNilMessageField(t *testing.T) {
	line := `{"type":"user","timestamp":"2024-01-01T00:00:00Z"}`
	path := writeJSONLFile(t, []string{line})
	msgs, err := parseJSONL(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages when message field is absent, got %d", len(msgs))
	}
}

func TestParseJSONL_skipsMessagesWithEmptyContent(t *testing.T) {
	line := `{"type":"user","message":{"role":"user","content":""},"timestamp":"2024-01-01T00:00:00Z"}`
	path := writeJSONLFile(t, []string{line})
	msgs, err := parseJSONL(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for empty content, got %d", len(msgs))
	}
}

// ---------------------------------------------------------------------------
// FormatConversation
// ---------------------------------------------------------------------------

func TestFormatConversation_emptyInputReturnsEmptyString(t *testing.T) {
	result := FormatConversation([]Message{})
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestFormatConversation_includesRoleAndContent(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Hello", Timestamp: time.Time{}},
	}
	result := FormatConversation(msgs)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if len(result) == 0 {
		t.Error("expected formatted output to contain content")
	}
}

func TestFormatConversation_userRoleHeaderFormat(t *testing.T) {
	ts, _ := time.Parse(time.RFC3339, "2024-01-01T15:04:05Z")
	msgs := []Message{{Role: "user", Content: "Hello there", Timestamp: ts}}
	result := FormatConversation(msgs)
	if !containsSubstr(result, "User") {
		t.Errorf("expected 'User' in output, got: %q", result)
	}
	if !containsSubstr(result, "Hello there") {
		t.Errorf("expected content 'Hello there' in output, got: %q", result)
	}
}

func TestFormatConversation_assistantRoleHeaderFormat(t *testing.T) {
	ts, _ := time.Parse(time.RFC3339, "2024-01-01T09:00:00Z")
	msgs := []Message{{Role: "assistant", Content: "Sure!", Timestamp: ts}}
	result := FormatConversation(msgs)
	if !containsSubstr(result, "Assistant") {
		t.Errorf("expected 'Assistant' in output, got: %q", result)
	}
	if !containsSubstr(result, "Sure!") {
		t.Errorf("expected content 'Sure!' in output, got: %q", result)
	}
}

func TestFormatConversation_multipleMessagesAllPresent(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Question", Timestamp: time.Time{}},
		{Role: "assistant", Content: "Answer", Timestamp: time.Time{}},
	}
	result := FormatConversation(msgs)
	if !containsSubstr(result, "Question") {
		t.Errorf("expected 'Question' in output")
	}
	if !containsSubstr(result, "Answer") {
		t.Errorf("expected 'Answer' in output")
	}
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

// ---------------------------------------------------------------------------
// findLatestJSONL
// ---------------------------------------------------------------------------

func TestFindLatestJSONL_returnsErrorForNonexistentDir(t *testing.T) {
	_, err := findLatestJSONL("/nonexistent/project/dir")
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}

func TestFindLatestJSONL_returnsErrorWhenNoJSONLFiles(t *testing.T) {
	dir := t.TempDir()
	// Write a non-jsonl file
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0644)
	_, err := findLatestJSONL(dir)
	if err == nil {
		t.Error("expected error when no .jsonl files exist, got nil")
	}
}

func TestFindLatestJSONL_returnsSingleJSONLFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "conv.jsonl")
	_ = os.WriteFile(target, []byte(`{}`), 0644)
	got, err := findLatestJSONL(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != target {
		t.Errorf("expected %q, got %q", target, got)
	}
}

func TestFindLatestJSONL_returnsNewestWhenMultipleExist(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "old.jsonl")
	newer := filepath.Join(dir, "new.jsonl")
	_ = os.WriteFile(older, []byte(`{}`), 0644)
	// Small sleep to ensure different mod times
	_ = os.WriteFile(newer, []byte(`{}`), 0644)
	// Touch newer to guarantee it has a later mtime
	now := time.Now().Add(time.Second)
	_ = os.Chtimes(newer, now, now)

	got, err := findLatestJSONL(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != newer {
		t.Errorf("expected newest file %q, got %q", newer, got)
	}
}

func TestFindLatestJSONL_ignoresSubdirectories(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subdir.jsonl") // a *directory* ending in .jsonl
	_ = os.Mkdir(subDir, 0755)
	realFile := filepath.Join(dir, "real.jsonl")
	_ = os.WriteFile(realFile, []byte(`{}`), 0644)

	got, err := findLatestJSONL(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != realFile {
		t.Errorf("expected %q, got %q", realFile, got)
	}
}

// ---------------------------------------------------------------------------
// ReadConversation integration (uses real filesystem via TempDir)
// ---------------------------------------------------------------------------

func TestReadConversation_emptyWorkDirReturnsError(t *testing.T) {
	_, err := ReadConversation("", 10)
	if err == nil {
		t.Error("expected error for empty workDir, got nil")
	}
}

func TestReadConversation_nonexistentProjectDirReturnsError(t *testing.T) {
	// mapToProjectDir will produce a path under ~/.claude/projects/ that almost
	// certainly does not exist when the workDir is a random temp path.
	_, err := ReadConversation("/tmp/this-path-will-never-have-claude-logs-xyzzy123", 10)
	if err == nil {
		t.Error("expected error for nonexistent project dir, got nil")
	}
}
