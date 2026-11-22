package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// settingsPath returns a fresh settings.json path inside t.TempDir().
func settingsPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, ".claude", "settings.json")
}

// writeFile writes content to path, creating parent dirs as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// readHooksMap parses the settings file at path and returns its hooks sub-map.
func readHooksMap(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	top, _, err := loadSettings(path)
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	hm, err := getHooksMap(top)
	if err != nil {
		t.Fatalf("getHooksMap: %v", err)
	}
	return hm
}

// assertPresent fails the test if e is not found in hooksMap.
func assertPresent(t *testing.T, hooksMap map[string]json.RawMessage, e Entry) {
	t.Helper()
	if !entryPresent(hooksMap, e) {
		t.Errorf("expected entry present: {Event:%s Matcher:%q Command:%s}", e.Event, e.Matcher, e.Command)
	}
}

// assertAbsent fails the test if e is found in hooksMap.
func assertAbsent(t *testing.T, hooksMap map[string]json.RawMessage, e Entry) {
	t.Helper()
	if entryPresent(hooksMap, e) {
		t.Errorf("expected entry absent: {Event:%s Matcher:%q Command:%s}", e.Event, e.Matcher, e.Command)
	}
}

// countGroupCommand counts how many times command appears in the event+matcher group.
func countGroupCommand(t *testing.T, hm map[string]json.RawMessage, event, matcher, command string) int {
	t.Helper()
	raw, ok := hm[event]
	if !ok {
		return 0
	}
	var groups []json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		t.Fatalf("unmarshal groups for %s: %v", event, err)
	}
	total := 0
	for _, g := range groups {
		m, err := rawGroupMatcher(g)
		if err != nil || m != matcher {
			continue
		}
		var grp struct {
			Hooks []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal(g, &grp); err != nil {
			t.Fatalf("unmarshal group: %v", err)
		}
		for _, h := range grp.Hooks {
			if h.Type == "command" && h.Command == command {
				total++
			}
		}
	}
	return total
}

// --- Tests ---

func TestEntries(t *testing.T) {
	entries := Entries()
	if len(entries) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(entries))
	}
	want := []Entry{
		{Event: "UserPromptSubmit", Matcher: "", Command: "claude-dashboard hook active"},
		{Event: "PostToolUse", Matcher: "*", Command: "claude-dashboard hook active"},
		{Event: "PermissionRequest", Matcher: "*", Command: "claude-dashboard hook waiting"},
		{Event: "Notification", Matcher: "elicitation_dialog", Command: "claude-dashboard hook asking"},
		{Event: "Stop", Matcher: "", Command: "claude-dashboard hook idle"},
		{Event: "SessionEnd", Matcher: "", Command: "claude-dashboard hook idle"},
	}
	index := map[string]bool{}
	for _, e := range entries {
		index[e.Event+"|"+e.Matcher+"|"+e.Command] = true
	}
	for _, e := range want {
		if !index[e.Event+"|"+e.Matcher+"|"+e.Command] {
			t.Errorf("missing entry: %+v", e)
		}
	}
}

func TestDefaultPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	got := DefaultPath()
	want := filepath.Join(tmp, ".claude", "settings.json")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestInstall_FreshFile(t *testing.T) {
	path := settingsPath(t)

	backup, err := Install(path)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if backup != "" {
		t.Errorf("expected no backup for fresh file, got %q", backup)
	}

	hm := readHooksMap(t, path)
	for _, e := range Entries() {
		assertPresent(t, hm, e)
	}
}

func TestInstall_Idempotent(t *testing.T) {
	path := settingsPath(t)

	if _, err := Install(path); err != nil {
		t.Fatalf("first Install: %v", err)
	}

	backup, err := Install(path)
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if backup != "" {
		t.Errorf("second Install should return empty backup (nothing changed), got %q", backup)
	}
}

func TestInstall_PreservesOtherKeys(t *testing.T) {
	path := settingsPath(t)
	writeFile(t, path, `{"model":"opus","theme":"dark","hooks":{}}`)

	if _, err := Install(path); err != nil {
		t.Fatalf("Install: %v", err)
	}

	top, _, err := loadSettings(path)
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	for _, key := range []string{"model", "theme"} {
		if _, ok := top[key]; !ok {
			t.Errorf("key %q missing after Install", key)
		}
	}
}

func TestInstall_CreatesBackupOnChange(t *testing.T) {
	path := settingsPath(t)
	// File exists but has no hooks yet.
	writeFile(t, path, `{}`)

	backup, err := Install(path)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if backup == "" {
		t.Fatal("expected backup path for existing file that changed, got empty")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Errorf("backup file missing: %v", err)
	}
}

func TestInstall_Migrate_OldNotification(t *testing.T) {
	path := settingsPath(t)
	writeFile(t, path, `{
		"hooks": {
			"Notification": [
				{
					"matcher": "elicitation_dialog",
					"hooks": [{"type":"command","command":"claude-dashboard hook waiting"}]
				}
			]
		}
	}`)

	if _, err := Install(path); err != nil {
		t.Fatalf("Install: %v", err)
	}

	hm := readHooksMap(t, path)
	assertPresent(t, hm, currentNotifEntry())
	assertAbsent(t, hm, legacyNotifEntry)
}

func TestInstall_Migrate_OtherHooksPreserved(t *testing.T) {
	path := settingsPath(t)
	// Group has the old command and an unrelated command.
	writeFile(t, path, `{
		"hooks": {
			"Notification": [
				{
					"matcher": "elicitation_dialog",
					"hooks": [
						{"type":"command","command":"claude-dashboard hook waiting"},
						{"type":"command","command":"some-other-tool notify"}
					]
				}
			]
		}
	}`)

	if _, err := Install(path); err != nil {
		t.Fatalf("Install: %v", err)
	}

	hm := readHooksMap(t, path)
	assertPresent(t, hm, currentNotifEntry())
	assertAbsent(t, hm, legacyNotifEntry)
	// Unrelated hook must still be present.
	other := Entry{Event: "Notification", Matcher: "elicitation_dialog", Command: "some-other-tool notify"}
	assertPresent(t, hm, other)
}

func TestInstall_Migrate_NoDuplicate(t *testing.T) {
	path := settingsPath(t)
	// Both old and new already present — old should be removed, new kept once.
	writeFile(t, path, `{
		"hooks": {
			"Notification": [
				{
					"matcher": "elicitation_dialog",
					"hooks": [
						{"type":"command","command":"claude-dashboard hook waiting"},
						{"type":"command","command":"claude-dashboard hook asking"}
					]
				}
			]
		}
	}`)

	if _, err := Install(path); err != nil {
		t.Fatalf("Install: %v", err)
	}

	hm := readHooksMap(t, path)
	assertPresent(t, hm, currentNotifEntry())
	assertAbsent(t, hm, legacyNotifEntry)

	// "asking" must appear exactly once in the group.
	count := countGroupCommand(t, hm, "Notification", "elicitation_dialog", "claude-dashboard hook asking")
	if count != 1 {
		t.Errorf("expected exactly 1 asking command, got %d", count)
	}
}

func TestInstall_Migrate_Idempotent(t *testing.T) {
	path := settingsPath(t)
	writeFile(t, path, `{
		"hooks": {
			"Notification": [
				{
					"matcher": "elicitation_dialog",
					"hooks": [{"type":"command","command":"claude-dashboard hook waiting"}]
				}
			]
		}
	}`)

	if _, err := Install(path); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	// Second Install: nothing should change.
	backup, err := Install(path)
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if backup != "" {
		t.Errorf("second Install should return empty backup (nothing changed), got %q", backup)
	}
}

func TestPlan_FreshFile(t *testing.T) {
	path := settingsPath(t)
	// File does not exist.

	plan, err := Plan(path)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !strings.HasPrefix(plan, "would:") {
		t.Errorf("expected plan to start with 'would:', got %q", plan)
	}
	for _, e := range Entries() {
		if !strings.Contains(plan, e.Command) {
			t.Errorf("plan missing command %q:\n%s", e.Command, plan)
		}
	}
}

func TestPlan_AlreadyInstalled(t *testing.T) {
	path := settingsPath(t)
	if _, err := Install(path); err != nil {
		t.Fatalf("Install: %v", err)
	}

	plan, err := Plan(path)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan != "already installed" {
		t.Errorf("expected 'already installed', got %q", plan)
	}
}

func TestPlan_MigrationCase(t *testing.T) {
	path := settingsPath(t)
	writeFile(t, path, `{
		"hooks": {
			"Notification": [
				{
					"matcher": "elicitation_dialog",
					"hooks": [{"type":"command","command":"claude-dashboard hook waiting"}]
				}
			]
		}
	}`)

	plan, err := Plan(path)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !strings.Contains(plan, "will change") {
		t.Errorf("expected 'will change' in plan, got %q", plan)
	}
	if !strings.Contains(plan, "waiting") || !strings.Contains(plan, "asking") {
		t.Errorf("plan should mention both 'waiting' and 'asking':\n%s", plan)
	}
}

func TestPlan_PartialInstall(t *testing.T) {
	path := settingsPath(t)
	// UserPromptSubmit already installed; others are not.
	writeFile(t, path, `{
		"hooks": {
			"UserPromptSubmit": [
				{"hooks": [{"type":"command","command":"claude-dashboard hook active"}]}
			]
		}
	}`)

	plan, err := Plan(path)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !strings.HasPrefix(plan, "would:") {
		t.Errorf("expected plan to start with 'would:', got %q", plan)
	}
	// UserPromptSubmit is already present — plan must not list it as "add".
	if strings.Contains(plan, "add UserPromptSubmit") {
		t.Errorf("plan should not re-add UserPromptSubmit:\n%s", plan)
	}
}

func TestUninstall_RemovesAll(t *testing.T) {
	path := settingsPath(t)
	if _, err := Install(path); err != nil {
		t.Fatalf("Install: %v", err)
	}

	backup, err := Uninstall(path)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if backup == "" {
		t.Fatal("expected backup path, got empty")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Errorf("backup file missing: %v", err)
	}

	hm := readHooksMap(t, path)
	for _, e := range Entries() {
		assertAbsent(t, hm, e)
	}
}

func TestUninstall_RemovesBothForms_OldForm(t *testing.T) {
	path := settingsPath(t)
	// Only the legacy "waiting" entry is present.
	writeFile(t, path, `{
		"hooks": {
			"Notification": [
				{
					"matcher": "elicitation_dialog",
					"hooks": [{"type":"command","command":"claude-dashboard hook waiting"}]
				}
			]
		}
	}`)

	if _, err := Uninstall(path); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	hm := readHooksMap(t, path)
	assertAbsent(t, hm, legacyNotifEntry)
	assertAbsent(t, hm, currentNotifEntry())
}

func TestUninstall_RemovesBothForms_NewForm(t *testing.T) {
	path := settingsPath(t)
	// Install first to get the new "asking" entry.
	if _, err := Install(path); err != nil {
		t.Fatalf("Install: %v", err)
	}

	hm := readHooksMap(t, path)
	assertPresent(t, hm, currentNotifEntry())

	if _, err := Uninstall(path); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	hm = readHooksMap(t, path)
	assertAbsent(t, hm, currentNotifEntry())
	assertAbsent(t, hm, legacyNotifEntry)
}

func TestUninstall_RemovesBothForms_BothPresent(t *testing.T) {
	path := settingsPath(t)
	// Both old and new forms are in the group simultaneously.
	writeFile(t, path, `{
		"hooks": {
			"Notification": [
				{
					"matcher": "elicitation_dialog",
					"hooks": [
						{"type":"command","command":"claude-dashboard hook waiting"},
						{"type":"command","command":"claude-dashboard hook asking"}
					]
				}
			]
		}
	}`)

	if _, err := Uninstall(path); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	hm := readHooksMap(t, path)
	assertAbsent(t, hm, legacyNotifEntry)
	assertAbsent(t, hm, currentNotifEntry())
}

func TestUninstall_NonExistentFile(t *testing.T) {
	path := settingsPath(t)
	// File does not exist.

	backup, err := Uninstall(path)
	if err != nil {
		t.Fatalf("Uninstall on non-existent file returned error: %v", err)
	}
	if backup != "" {
		t.Errorf("expected empty backup for non-existent file, got %q", backup)
	}
}

func TestInstall_InvalidJSON(t *testing.T) {
	path := settingsPath(t)
	writeFile(t, path, `{not valid json}`)

	_, err := Install(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
