// Package hooks manages claude-dashboard's hook entries in Claude Code's settings.json.
//
// Key ordering note: when settings.json is rewritten, top-level and nested object
// keys are sorted alphabetically by encoding/json. Values that are not hooks-related
// are preserved verbatim via json.RawMessage.
package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const hookCmd = "claude-dashboard hook"

// legacyNotifEntry is the deprecated Notification hook that Install migrates in place.
var legacyNotifEntry = Entry{
	Event:   "Notification",
	Matcher: "elicitation_dialog",
	Command: hookCmd + " waiting",
}

// Entry describes one hook entry that claude-dashboard registers in Claude Code's settings.json.
type Entry struct {
	Event   string // e.g. "UserPromptSubmit"
	Matcher string // empty means no matcher field
	Command string // e.g. "claude-dashboard hook active"
}

// Entries returns the hook entries claude-dashboard needs, in a stable order.
func Entries() []Entry {
	return []Entry{
		{Event: "UserPromptSubmit", Matcher: "", Command: hookCmd + " active"},
		{Event: "PostToolUse", Matcher: "*", Command: hookCmd + " active"},
		{Event: "PermissionRequest", Matcher: "*", Command: hookCmd + " waiting"},
		{Event: "Notification", Matcher: "elicitation_dialog", Command: hookCmd + " asking"},
		{Event: "Stop", Matcher: "", Command: hookCmd + " idle"},
		{Event: "SessionEnd", Matcher: "", Command: hookCmd + " idle"},
	}
}

// DefaultPath returns the default Claude Code settings path (~/.claude/settings.json).
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// Plan reports what Install would do without making any changes.
// Returns "already installed" if nothing would change.
func Plan(path string) (string, error) {
	top, _, err := loadSettings(path)
	if err != nil {
		return "", err
	}
	hooksMap, err := getHooksMap(top)
	if err != nil {
		return "", err
	}
	if err := validateEventArrays(hooksMap, Entries()); err != nil {
		return "", err
	}

	var lines []string

	hasOld := entryPresent(hooksMap, legacyNotifEntry)
	hasNew := entryPresent(hooksMap, currentNotifEntry())
	if hasOld && !hasNew {
		lines = append(lines, fmt.Sprintf(
			"  will change Notification (matcher %q) %s → %s",
			legacyNotifEntry.Matcher, legacyNotifEntry.Command, currentNotifEntry().Command))
	}

	for _, e := range Entries() {
		if entryPresent(hooksMap, e) {
			continue
		}
		// Skip if covered by the migration line above.
		if e.Event == "Notification" && e.Matcher == "elicitation_dialog" && hasOld && !hasNew {
			continue
		}
		if e.Matcher != "" {
			lines = append(lines, fmt.Sprintf("  add %s (matcher %q) → %s", e.Event, e.Matcher, e.Command))
		} else {
			lines = append(lines, fmt.Sprintf("  add %s → %s", e.Event, e.Command))
		}
	}

	if len(lines) == 0 {
		return "already installed", nil
	}
	return "would:\n" + strings.Join(lines, "\n"), nil
}

// Install adds claude-dashboard's hook entries to the settings file at path.
// If the old Notification+elicitation_dialog entry using "waiting" exists it is
// migrated in-place to the current "asking" command before any new entries are added.
// Returns the path of the timestamped backup (empty if no file existed or nothing changed).
func Install(path string) (backup string, err error) {
	top, origBytes, err := loadSettings(path)
	if err != nil {
		return "", err
	}
	fileExisted := origBytes != nil

	hooksMap, err := getHooksMap(top)
	if err != nil {
		return "", err
	}
	if err := validateEventArrays(hooksMap, Entries()); err != nil {
		return "", err
	}

	changed := false

	migrated, err := migrateNotification(hooksMap)
	if err != nil {
		return "", fmt.Errorf("migrating Notification hook: %w", err)
	}
	if migrated {
		changed = true
	}

	for _, e := range Entries() {
		if entryPresent(hooksMap, e) {
			continue
		}
		if err := addEntry(hooksMap, e); err != nil {
			return "", fmt.Errorf("adding entry {%s %q %s}: %w", e.Event, e.Matcher, e.Command, err)
		}
		changed = true
	}

	if !changed {
		return "", nil
	}

	hooksRaw, err := json.Marshal(hooksMap)
	if err != nil {
		return "", fmt.Errorf("encoding hooks: %w", err)
	}
	top["hooks"] = json.RawMessage(hooksRaw)

	if fileExisted {
		backup, err = writeBackup(path, origBytes)
		if err != nil {
			return "", fmt.Errorf("writing backup: %w", err)
		}
	}

	if err := writeAtomic(path, top); err != nil {
		return backup, fmt.Errorf("writing settings: %w", err)
	}
	return backup, nil
}

// Uninstall removes claude-dashboard's hook entries from the settings file at path.
// Both the current Notification entry ("asking") and the legacy one ("waiting") are removed.
// Returns the path of the timestamped backup (empty if no file existed or nothing changed).
func Uninstall(path string) (backup string, err error) {
	top, origBytes, err := loadSettings(path)
	if err != nil {
		return "", err
	}
	if origBytes == nil {
		return "", nil
	}

	hooksMap, err := getHooksMap(top)
	if err != nil {
		return "", err
	}
	if err := validateEventArrays(hooksMap, Entries()); err != nil {
		return "", err
	}

	changed := false

	for _, e := range Entries() {
		if !entryPresent(hooksMap, e) {
			continue
		}
		if err := removeEntry(hooksMap, e); err != nil {
			return "", fmt.Errorf("removing entry {%s %q %s}: %w", e.Event, e.Matcher, e.Command, err)
		}
		changed = true
	}

	// Also remove legacy Notification entry if still present.
	if entryPresent(hooksMap, legacyNotifEntry) {
		if err := removeEntry(hooksMap, legacyNotifEntry); err != nil {
			return "", fmt.Errorf("removing legacy entry: %w", err)
		}
		changed = true
	}

	if !changed {
		return "", nil
	}

	hooksRaw, err := json.Marshal(hooksMap)
	if err != nil {
		return "", fmt.Errorf("encoding hooks: %w", err)
	}
	top["hooks"] = json.RawMessage(hooksRaw)

	backup, err = writeBackup(path, origBytes)
	if err != nil {
		return "", fmt.Errorf("writing backup: %w", err)
	}

	if err := writeAtomic(path, top); err != nil {
		return backup, fmt.Errorf("writing settings: %w", err)
	}
	return backup, nil
}

// currentNotifEntry returns the Notification+elicitation_dialog entry from Entries().
func currentNotifEntry() Entry {
	for _, e := range Entries() {
		if e.Event == "Notification" && e.Matcher == "elicitation_dialog" {
			return e
		}
	}
	panic("hooks: Notification+elicitation_dialog not found in Entries()")
}

// loadSettings reads and parses the settings file.
// If the file does not exist, returns an empty map and nil bytes without error.
func loadSettings(path string) (map[string]json.RawMessage, []byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]json.RawMessage{}, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return top, data, nil
}

// getHooksMap returns the "hooks" object from top, creating an empty one if absent.
// Returns an error if "hooks" exists but is not a JSON object.
func getHooksMap(top map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	raw, ok := top["hooks"]
	if !ok {
		return map[string]json.RawMessage{}, nil
	}
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &hooksMap); err != nil {
		return nil, fmt.Errorf("\"hooks\" is not a JSON object: %w", err)
	}
	if hooksMap == nil {
		return map[string]json.RawMessage{}, nil
	}
	return hooksMap, nil
}

// validateEventArrays verifies that every event in entries that already exists in
// hooksMap is stored as a JSON array. Called before any writes so errors abort early.
func validateEventArrays(hooksMap map[string]json.RawMessage, entries []Entry) error {
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.Event] {
			continue
		}
		seen[e.Event] = true
		raw, ok := hooksMap[e.Event]
		if !ok {
			continue
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return fmt.Errorf("\"hooks.%s\" is not a JSON array: %w", e.Event, err)
		}
	}
	return nil
}

// entryPresent reports whether e exists in hooksMap.
func entryPresent(hooksMap map[string]json.RawMessage, e Entry) bool {
	raw, ok := hooksMap[e.Event]
	if !ok {
		return false
	}
	var groups []json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		return false
	}
	for _, g := range groups {
		m, err := rawGroupMatcher(g)
		if err != nil || m != e.Matcher {
			continue
		}
		has, err := groupHasCommand(g, e.Command)
		if err == nil && has {
			return true
		}
	}
	return false
}

// addEntry appends e to hooksMap. If a group with the same matcher already exists,
// the command is appended to it; otherwise a new group is created.
func addEntry(hooksMap map[string]json.RawMessage, e Entry) error {
	hookRaw, err := json.Marshal(struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}{"command", e.Command})
	if err != nil {
		return err
	}

	var groups []json.RawMessage
	if raw, ok := hooksMap[e.Event]; ok {
		if err := json.Unmarshal(raw, &groups); err != nil {
			return fmt.Errorf("unexpected: event %q is not an array", e.Event)
		}
	}

	for i, g := range groups {
		m, err := rawGroupMatcher(g)
		if err != nil || m != e.Matcher {
			continue
		}
		newG, err := appendToGroup(g, hookRaw)
		if err != nil {
			return err
		}
		groups[i] = newG
		arr, err := json.Marshal(groups)
		if err != nil {
			return err
		}
		hooksMap[e.Event] = json.RawMessage(arr)
		return nil
	}

	newG, err := buildGroup(e.Matcher, []json.RawMessage{hookRaw})
	if err != nil {
		return err
	}
	groups = append(groups, newG)
	arr, err := json.Marshal(groups)
	if err != nil {
		return err
	}
	hooksMap[e.Event] = json.RawMessage(arr)
	return nil
}

// removeEntry removes e from hooksMap. Empty groups and events are pruned.
func removeEntry(hooksMap map[string]json.RawMessage, e Entry) error {
	raw, ok := hooksMap[e.Event]
	if !ok {
		return nil
	}
	var groups []json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil
	}

	var newGroups []json.RawMessage
	for _, g := range groups {
		m, err := rawGroupMatcher(g)
		if err != nil || m != e.Matcher {
			newGroups = append(newGroups, g)
			continue
		}
		newG, removed, err := removeCommandFromGroup(g, e.Command)
		if err != nil {
			return err
		}
		if !removed {
			newGroups = append(newGroups, g)
			continue
		}
		if newG != nil {
			newGroups = append(newGroups, newG)
		}
		// nil newG means the group became empty; drop it.
	}

	if len(newGroups) == 0 {
		delete(hooksMap, e.Event)
	} else {
		arr, err := json.Marshal(newGroups)
		if err != nil {
			return err
		}
		hooksMap[e.Event] = json.RawMessage(arr)
	}
	return nil
}

// migrateNotification converts the legacy "claude-dashboard hook waiting" command in
// the Notification+elicitation_dialog group to "claude-dashboard hook asking", in place.
// If "asking" is already present the old command is simply removed (no duplicate).
// Returns (changed, error).
func migrateNotification(hooksMap map[string]json.RawMessage) (bool, error) {
	raw, ok := hooksMap["Notification"]
	if !ok {
		return false, nil
	}
	var groups []json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		return false, nil
	}

	changed := false
	for i, g := range groups {
		m, err := rawGroupMatcher(g)
		if err != nil || m != "elicitation_dialog" {
			continue
		}
		newG, did, err := replaceOrRemoveCommandInGroup(g, legacyNotifEntry.Command, currentNotifEntry().Command)
		if err != nil {
			return false, err
		}
		if did {
			groups[i] = newG
			changed = true
		}
	}

	if !changed {
		return false, nil
	}

	arr, err := json.Marshal(groups)
	if err != nil {
		return false, err
	}
	hooksMap["Notification"] = json.RawMessage(arr)
	return true, nil
}

// rawGroupMatcher extracts the "matcher" field from a group object.
// Returns "" when the field is absent.
func rawGroupMatcher(raw json.RawMessage) (string, error) {
	var g struct {
		Matcher *string `json:"matcher"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		return "", err
	}
	if g.Matcher == nil {
		return "", nil
	}
	return *g.Matcher, nil
}

// groupHasCommand reports whether the group contains a hook with type="command"
// and the given command string.
func groupHasCommand(raw json.RawMessage, command string) (bool, error) {
	var g struct {
		Hooks []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		return false, err
	}
	for _, h := range g.Hooks {
		if h.Type == "command" && h.Command == command {
			return true, nil
		}
	}
	return false, nil
}

// buildGroup constructs a new group JSON object with the given matcher and hooks slice.
func buildGroup(matcher string, hooks []json.RawMessage) (json.RawMessage, error) {
	g := map[string]json.RawMessage{}
	if matcher != "" {
		m, err := json.Marshal(matcher)
		if err != nil {
			return nil, err
		}
		g["matcher"] = json.RawMessage(m)
	}
	h, err := json.Marshal(hooks)
	if err != nil {
		return nil, err
	}
	g["hooks"] = json.RawMessage(h)
	return json.Marshal(g)
}

// appendToGroup appends hookRaw to the group's "hooks" array.
func appendToGroup(groupRaw json.RawMessage, hookRaw json.RawMessage) (json.RawMessage, error) {
	var g map[string]json.RawMessage
	if err := json.Unmarshal(groupRaw, &g); err != nil {
		return nil, err
	}
	var hooks []json.RawMessage
	if raw, ok := g["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, err
		}
	}
	hooks = append(hooks, hookRaw)
	hRaw, err := json.Marshal(hooks)
	if err != nil {
		return nil, err
	}
	g["hooks"] = json.RawMessage(hRaw)
	return json.Marshal(g)
}

// removeCommandFromGroup removes the first hook with type="command" and the given
// command from the group's "hooks" array.
// Returns (modified group, true, nil) if removed; (original, false, nil) if not found.
// Returns (nil, true, nil) when the hooks array becomes empty after removal.
func removeCommandFromGroup(groupRaw json.RawMessage, command string) (json.RawMessage, bool, error) {
	var g map[string]json.RawMessage
	if err := json.Unmarshal(groupRaw, &g); err != nil {
		return groupRaw, false, nil
	}
	hookRaw, ok := g["hooks"]
	if !ok {
		return groupRaw, false, nil
	}
	var hooks []json.RawMessage
	if err := json.Unmarshal(hookRaw, &hooks); err != nil {
		return groupRaw, false, nil
	}

	var kept []json.RawMessage
	removed := false
	for _, h := range hooks {
		var hv struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		}
		if err := json.Unmarshal(h, &hv); err != nil {
			kept = append(kept, h)
			continue
		}
		if !removed && hv.Type == "command" && hv.Command == command {
			removed = true
			continue
		}
		kept = append(kept, h)
	}

	if !removed {
		return groupRaw, false, nil
	}
	if len(kept) == 0 {
		return nil, true, nil
	}

	hRaw, err := json.Marshal(kept)
	if err != nil {
		return nil, false, err
	}
	g["hooks"] = json.RawMessage(hRaw)
	raw, err := json.Marshal(g)
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(raw), true, nil
}

// replaceOrRemoveCommandInGroup replaces oldCmd with newCmd in the group's "hooks" array,
// at the same position (in-place). If newCmd is already present, oldCmd is simply removed
// to avoid a duplicate. Returns (modified group, true, nil) if changed; (original, false, nil)
// if oldCmd is not found.
func replaceOrRemoveCommandInGroup(groupRaw json.RawMessage, oldCmd, newCmd string) (json.RawMessage, bool, error) {
	var g map[string]json.RawMessage
	if err := json.Unmarshal(groupRaw, &g); err != nil {
		return groupRaw, false, nil
	}
	hookRaw, ok := g["hooks"]
	if !ok {
		return groupRaw, false, nil
	}
	var hooks []json.RawMessage
	if err := json.Unmarshal(hookRaw, &hooks); err != nil {
		return groupRaw, false, nil
	}

	hasOld := false
	for _, h := range hooks {
		var hv struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		}
		if json.Unmarshal(h, &hv) == nil && hv.Type == "command" && hv.Command == oldCmd {
			hasOld = true
			break
		}
	}
	if !hasOld {
		return groupRaw, false, nil
	}

	hasNew := false
	for _, h := range hooks {
		var hv struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		}
		if json.Unmarshal(h, &hv) == nil && hv.Type == "command" && hv.Command == newCmd {
			hasNew = true
			break
		}
	}

	newHookRaw, err := json.Marshal(struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}{"command", newCmd})
	if err != nil {
		return groupRaw, false, err
	}

	alreadyReplaced := false
	var kept []json.RawMessage
	for _, h := range hooks {
		var hv struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		}
		if err := json.Unmarshal(h, &hv); err != nil {
			kept = append(kept, h)
			continue
		}
		if hv.Type == "command" && hv.Command == oldCmd {
			if !alreadyReplaced && !hasNew {
				kept = append(kept, newHookRaw) // replace in-place
				alreadyReplaced = true
			}
			// Drop old command whether we replaced or not.
			continue
		}
		kept = append(kept, h)
	}

	hRaw, err := json.Marshal(kept)
	if err != nil {
		return groupRaw, false, err
	}
	g["hooks"] = json.RawMessage(hRaw)
	raw, err := json.Marshal(g)
	if err != nil {
		return groupRaw, false, err
	}
	return json.RawMessage(raw), true, nil
}

// writeBackup saves content to a timestamped backup path alongside the original file.
func writeBackup(path string, content []byte) (string, error) {
	ts := time.Now().Format("20060102-150405")
	backupPath := path + ".claude-dashboard-bak-" + ts
	if err := os.WriteFile(backupPath, content, 0600); err != nil {
		return "", err
	}
	return backupPath, nil
}

// writeAtomic encodes top to JSON and writes it to path atomically via a temp file rename.
// The directory is created if it does not exist.
func writeAtomic(path string, top map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".settings-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming to %s: %w", path, err)
	}
	return nil
}
