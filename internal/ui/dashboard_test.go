package ui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate_shortStringIsUnchanged(t *testing.T) {
	got := truncate("hello", 10)
	if got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestTruncate_exactLengthIsUnchanged(t *testing.T) {
	got := truncate("hello", 5)
	if got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestTruncate_longStringIsTruncatedWithEllipsis(t *testing.T) {
	got := truncate("hello world", 8)
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected truncated string to end with '...', got %q", got)
	}
	if len(got) > 8 {
		t.Errorf("expected length <= 8, got %d (%q)", len(got), got)
	}
}

func TestTruncate_maxLenThreeReturnsTruncatedWithoutEllipsis(t *testing.T) {
	// When maxLen <= 3, the function returns s[:maxLen] without "..."
	got := truncate("abcdef", 3)
	if got != "abc" {
		t.Errorf("expected %q, got %q", "abc", got)
	}
}

func TestTruncate_maxLenOneReturnsSingleChar(t *testing.T) {
	got := truncate("abcdef", 1)
	if got != "a" {
		t.Errorf("expected %q, got %q", "a", got)
	}
}

func TestTruncate_emptyStringIsUnchanged(t *testing.T) {
	got := truncate("", 5)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestTruncate_ellipsisIsAddedForMaxLenFour(t *testing.T) {
	// maxLen=4 > 3, so should truncate to s[:1] + "..."
	got := truncate("abcdefgh", 4)
	if got != "a..." {
		t.Errorf("expected %q, got %q", "a...", got)
	}
}

// ---------------------------------------------------------------------------
// truncatePath
// ---------------------------------------------------------------------------

func TestTruncatePath_shortPathIsUnchanged(t *testing.T) {
	got := truncatePath("/a/b", 20)
	if got != "/a/b" {
		t.Errorf("expected %q, got %q", "/a/b", got)
	}
}

func TestTruncatePath_exactLengthIsUnchanged(t *testing.T) {
	s := "/a/b/c"
	got := truncatePath(s, len(s))
	if got != s {
		t.Errorf("expected %q, got %q", s, got)
	}
}

func TestTruncatePath_longPathTruncatesFromLeft(t *testing.T) {
	got := truncatePath("/very/long/path/to/some/directory", 15)
	if !strings.HasPrefix(got, "...") {
		t.Errorf("expected truncated path to start with '...', got %q", got)
	}
}

func TestTruncatePath_maxLenZeroReturnsOriginal(t *testing.T) {
	got := truncatePath("/a/b/c", 0)
	if got != "/a/b/c" {
		t.Errorf("expected original path when maxLen=0, got %q", got)
	}
}

func TestTruncatePath_maxLenNegativeReturnsOriginal(t *testing.T) {
	got := truncatePath("/a/b/c", -1)
	if got != "/a/b/c" {
		t.Errorf("expected original path when maxLen<0, got %q", got)
	}
}

func TestTruncatePath_maxLenThreeReturnsEllipsis(t *testing.T) {
	got := truncatePath("/some/very/long/path", 3)
	if got != "..." {
		t.Errorf("expected %q, got %q", "...", got)
	}
}

func TestTruncatePath_maxLenOneReturnsEllipsis(t *testing.T) {
	got := truncatePath("/some/path", 1)
	// maxLen <= 3, so return "..."
	if got != "..." {
		t.Errorf("expected %q, got %q", "...", got)
	}
}

func TestTruncatePath_emptyStringIsUnchanged(t *testing.T) {
	got := truncatePath("", 10)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestTruncatePath_truncatedLengthRespectsBound(t *testing.T) {
	s := "/home/user/projects/myapp/src/components/button.go"
	maxLen := 20
	got := truncatePath(s, maxLen)
	if len(got) > maxLen {
		t.Errorf("expected length <= %d, got %d (%q)", maxLen, len(got), got)
	}
}

// ---------------------------------------------------------------------------
// Table-driven tests combining truncate and truncatePath
// ---------------------------------------------------------------------------

func TestTruncateTableDriven(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"empty input", "", 5, ""},
		{"shorter than max", "hi", 10, "hi"},
		{"equal to max", "hello", 5, "hello"},
		{"longer, maxLen=4", "hello!", 4, "h..."},
		{"longer, maxLen=6", "hello world", 6, "hel..."},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.input, tc.maxLen)
			if got != tc.expected {
				t.Errorf("truncate(%q, %d): expected %q, got %q", tc.input, tc.maxLen, tc.expected, got)
			}
		})
	}
}

func TestTruncatePathTableDriven(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		maxLen int
		check  func(string) bool
		desc   string
	}{
		{"zero maxLen returns original", "/a/b", 0, func(s string) bool { return s == "/a/b" }, "should equal original"},
		{"short path unchanged", "/a/b", 10, func(s string) bool { return s == "/a/b" }, "should be unchanged"},
		{"long path starts with ...", "/a/b/c/d/e/f/g", 8, func(s string) bool { return strings.HasPrefix(s, "...") }, "should start with ..."},
		{"max 3 returns ...", "/long/path", 3, func(s string) bool { return s == "..." }, "should be exactly ..."},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := truncatePath(tc.input, tc.maxLen)
			if !tc.check(got) {
				t.Errorf("truncatePath(%q, %d): %s, got %q", tc.input, tc.maxLen, tc.desc, got)
			}
		})
	}
}
