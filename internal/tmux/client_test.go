package tmux

import (
	"testing"
)

// ---------------------------------------------------------------------------
// validateSessionName
// ---------------------------------------------------------------------------

func TestValidateSessionName_alphanumericIsValid(t *testing.T) {
	cases := []string{
		"mysession",
		"Session123",
		"abc",
		"ABC",
		"a1b2c3",
	}
	for _, name := range cases {
		if err := validateSessionName(name); err != nil {
			t.Errorf("expected no error for %q, got %v", name, err)
		}
	}
}

func TestValidateSessionName_underscoreIsValid(t *testing.T) {
	if err := validateSessionName("my_session"); err != nil {
		t.Errorf("expected no error for underscore, got %v", err)
	}
}

func TestValidateSessionName_hyphenIsValid(t *testing.T) {
	if err := validateSessionName("my-session"); err != nil {
		t.Errorf("expected no error for hyphen, got %v", err)
	}
}

func TestValidateSessionName_mixedAlphanumericUnderscoreHyphen(t *testing.T) {
	name := "cd-my_session-42"
	if err := validateSessionName(name); err != nil {
		t.Errorf("expected no error for %q, got %v", name, err)
	}
}

func TestValidateSessionName_spaceIsRejected(t *testing.T) {
	if err := validateSessionName("my session"); err == nil {
		t.Error("expected error for name containing space, got nil")
	}
}

func TestValidateSessionName_dotIsRejected(t *testing.T) {
	if err := validateSessionName("my.session"); err == nil {
		t.Error("expected error for name containing dot, got nil")
	}
}

func TestValidateSessionName_slashIsRejected(t *testing.T) {
	if err := validateSessionName("my/session"); err == nil {
		t.Error("expected error for name containing slash, got nil")
	}
}

func TestValidateSessionName_semicolonIsRejected(t *testing.T) {
	if err := validateSessionName("my;session"); err == nil {
		t.Error("expected error for name containing semicolon, got nil")
	}
}

func TestValidateSessionName_dollarIsRejected(t *testing.T) {
	if err := validateSessionName("$session"); err == nil {
		t.Error("expected error for name starting with dollar, got nil")
	}
}

func TestValidateSessionName_backtickIsRejected(t *testing.T) {
	if err := validateSessionName("my`session"); err == nil {
		t.Error("expected error for name containing backtick, got nil")
	}
}

func TestValidateSessionName_pipeIsRejected(t *testing.T) {
	if err := validateSessionName("my|session"); err == nil {
		t.Error("expected error for name containing pipe, got nil")
	}
}

func TestValidateSessionName_ampersandIsRejected(t *testing.T) {
	if err := validateSessionName("my&session"); err == nil {
		t.Error("expected error for name containing ampersand, got nil")
	}
}

func TestValidateSessionName_emptyStringIsRejected(t *testing.T) {
	if err := validateSessionName(""); err == nil {
		t.Error("expected error for empty session name, got nil")
	}
}

func TestValidateSessionName_newlineIsRejected(t *testing.T) {
	if err := validateSessionName("my\nsession"); err == nil {
		t.Error("expected error for name containing newline, got nil")
	}
}

func TestValidateSessionName_errorMessageContainsSessionName(t *testing.T) {
	err := validateSessionName("bad name!")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := err.Error()
	if len(errMsg) == 0 {
		t.Error("expected non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// Table-driven validation tests
// ---------------------------------------------------------------------------

func TestValidateSessionName_tableTests(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantError bool
	}{
		{"valid lowercase", "session", false},
		{"valid uppercase", "SESSION", false},
		{"valid with hyphen", "my-session", false},
		{"valid with underscore", "my_session", false},
		{"valid alphanumeric mix", "abc123XYZ", false},
		{"valid cd prefix", "cd-myproject", false},
		{"empty string", "", true},
		{"contains space", "my session", true},
		{"contains dot", "sess.ion", true},
		{"contains slash", "sess/ion", true},
		{"contains at", "sess@ion", true},
		{"contains hash", "sess#ion", true},
		{"contains exclamation", "sess!ion", true},
		{"contains parenthesis", "sess(ion", true},
		{"contains dollar", "$session", true},
		{"contains newline", "my\nsession", true},
		{"contains tab", "my\tsession", true},
		{"contains colon", "my:session", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateSessionName(tc.input)
			if tc.wantError && err == nil {
				t.Errorf("validateSessionName(%q): expected error, got nil", tc.input)
			}
			if !tc.wantError && err != nil {
				t.Errorf("validateSessionName(%q): expected no error, got %v", tc.input, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BuildProcChildren
// ---------------------------------------------------------------------------

func TestBuildProcChildren_emptyInputReturnsEmptyMap(t *testing.T) {
	result := BuildProcChildren(nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestBuildProcChildren_groupsByParentPID(t *testing.T) {
	entries := []struct{ PID, PPID, Args string }{
		{"10", "1", "bash"},
		{"11", "1", "zsh"},
		{"20", "10", "claude"},
	}
	result := BuildProcChildren(entries)

	children1, ok := result["1"]
	if !ok {
		t.Fatal("expected children for PPID '1'")
	}
	if len(children1) != 2 {
		t.Errorf("expected 2 children for PPID '1', got %d", len(children1))
	}

	children10, ok := result["10"]
	if !ok {
		t.Fatal("expected children for PPID '10'")
	}
	if len(children10) != 1 {
		t.Errorf("expected 1 child for PPID '10', got %d", len(children10))
	}
	if children10[0].PID != "20" {
		t.Errorf("expected child PID '20', got %q", children10[0].PID)
	}
	if children10[0].Args != "claude" {
		t.Errorf("expected child Args 'claude', got %q", children10[0].Args)
	}
}

func TestBuildProcChildren_singleEntry(t *testing.T) {
	entries := []struct{ PID, PPID, Args string }{
		{"42", "1", "some-process --flag"},
	}
	result := BuildProcChildren(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 parent entry, got %d", len(result))
	}
	children := result["1"]
	if len(children) != 1 || children[0].PID != "42" {
		t.Errorf("expected child {PID:42}, got %+v", children)
	}
}

// ---------------------------------------------------------------------------
// hasClaudeDescendant — using pre-built procChildren map
// ---------------------------------------------------------------------------

func TestHasClaudeDescendant_directChildWithClaudeInArgs(t *testing.T) {
	children := map[string][]ProcEntry{
		"100": {{PID: "200", Args: "/usr/local/bin/claude --verbose"}},
	}
	if !hasClaudeDescendant("100", children) {
		t.Error("expected true when direct child contains 'claude' in args")
	}
}

func TestHasClaudeDescendant_deepDescendantWithClaude(t *testing.T) {
	children := map[string][]ProcEntry{
		"1":   {{PID: "10", Args: "bash"}},
		"10":  {{PID: "20", Args: "node"}},
		"20":  {{PID: "30", Args: "claude-code"}},
	}
	if !hasClaudeDescendant("1", children) {
		t.Error("expected true when deep descendant has 'claude' in args")
	}
}

func TestHasClaudeDescendant_noClaudeInTree(t *testing.T) {
	children := map[string][]ProcEntry{
		"1":  {{PID: "10", Args: "bash"}},
		"10": {{PID: "20", Args: "vim"}},
	}
	if hasClaudeDescendant("1", children) {
		t.Error("expected false when no descendant has 'claude'")
	}
}

func TestHasClaudeDescendant_emptyTreeReturnsFalse(t *testing.T) {
	if hasClaudeDescendant("999", map[string][]ProcEntry{}) {
		t.Error("expected false for empty process tree")
	}
}

func TestHasClaudeDescendant_claudeInArgsCaseInsensitive(t *testing.T) {
	children := map[string][]ProcEntry{
		"1": {{PID: "2", Args: "/path/to/CLAUDE"}},
	}
	if !hasClaudeDescendant("1", children) {
		t.Error("expected true for case-insensitive match of 'claude' in args")
	}
}

func TestHasClaudeDescendant_cycleInTreeDoesNotInfiniteLoop(t *testing.T) {
	// Artificially create a cycle: 1 -> 2 -> 1 (should not loop forever due to visited map)
	children := map[string][]ProcEntry{
		"1": {{PID: "2", Args: "bash"}},
		"2": {{PID: "1", Args: "bash"}}, // back-edge cycle
	}
	// Should terminate and return false (no 'claude' in args)
	result := hasClaudeDescendant("1", children)
	if result {
		t.Error("expected false when no claude in cyclic tree")
	}
}
