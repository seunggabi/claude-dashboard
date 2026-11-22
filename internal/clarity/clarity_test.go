package clarity_test

import (
	"fmt"
	"testing"

	"github.com/seunggabi/claude-dashboard/internal/clarity"
)

// levelCase drives the table-driven level tests.
type levelCase struct {
	name  string
	text  string
	level string
}

var levelCases = []levelCase{
	{
		// Must be "clear": names two concrete targets (#123, acme/app, backtick),
		// contains outcome words ("tests", "pull request"), long enough.
		name:  "GitHub issue #123 with backtick and PR",
		text:  "Work on GitHub issue #123 in acme/app: read it with `gh issue view 123`, create a new branch, implement it with tests, and open a pull request that closes it.",
		level: "clear",
	},
	{
		// Must be "unclear": only 3 words (< 4), vague verb "improve" with no target.
		name:  "improve the code",
		text:  "improve the code",
		level: "unclear",
	},
	{
		// Must be "unclear": Korean, 11 runes (< 12), hedge words 대충/알아서.
		name:  "Korean vague 대충 알아서 정리해줘",
		text:  "대충 알아서 정리해줘",
		level: "unclear",
	},
	{
		// Must be "unclear": hedge phrases "make it better" and "somehow".
		name:  "make it better somehow",
		text:  "make it better somehow",
		level: "unclear",
	},
	{
		// Must be "clear": backtick target, outcome word "test", scope "without",
		// action verb "fix".
		name:  "fix failing test with backtick path and scope",
		text:  "fix the failing test in `internal/setup/setup_test.go` without changing the public API",
		level: "clear",
	},
	{
		// Must be "clear": Korean, backtick target, outcome words 테스트/통과,
		// action verb 리팩터.
		name:  "Korean clear – refactor with test outcome",
		text:  "`internal/setup/setup.go`의 Parse 함수를 리팩터링해서 테스트가 통과하도록 해주세요",
		level: "clear",
	},
	{
		// Must be "unclear": Korean, 10 runes (< 12), hedge 그냥.
		name:  "Korean unclear – 그냥 코드 개선해줘",
		text:  "그냥 코드 개선해줘",
		level: "unclear",
	},
	{
		// Must be "unclear": empty string is 0 words (< 4).
		name:  "empty string",
		text:  "",
		level: "unclear",
	},
	{
		// Must be "unclear": question (?) and only 3 words (< 4).
		name:  "is this correct?",
		text:  "is this correct?",
		level: "unclear",
	},
	{
		// Must be "vague": action verb bonus only; no target, no outcome, no scope.
		name:  "refactor the user service",
		text:  "refactor the user service",
		level: "vague",
	},
	{
		// Must be "unclear": single word (< 4), hedge "tbd".
		name:  "TBD alone",
		text:  "TBD",
		level: "unclear",
	},
	{
		// Must be "clear": backtick target, outcome "tests"/"must"/"pass",
		// action verb "implement".
		name:  "implement OAuth2 with must pass",
		text:  "implement OAuth2 login in `auth/oauth.go` by following RFC 6749, all existing tests must pass",
		level: "clear",
	},
	{
		// Must be "vague": Korean, > 12 runes, action verb 업데이트, but no
		// concrete target and no outcome word.
		name:  "Korean vague – server config update",
		text:  "서버 설정 파일 업데이트해줘",
		level: "vague",
	},
	{
		// Must be "clear": backtick target, outcome "tests", action verb "add".
		name:  "add tests for login handler with backtick path",
		text:  "add tests for the login handler in `handlers/login_test.go`",
		level: "clear",
	},
}

func TestScore_Level(t *testing.T) {
	for _, tc := range levelCases {
		t.Run(tc.name, func(t *testing.T) {
			got := clarity.Score(tc.text)
			if got.Level != tc.level {
				t.Errorf("Score(%q):\n  got  level=%q score=%d\n  want level=%q\n  reasons: %v",
					tc.text, got.Level, got.Score, tc.level, got.Reasons)
			}
		})
	}
}

// TestScore_Deterministic verifies the same input always yields the same output.
func TestScore_Deterministic(t *testing.T) {
	inputs := []string{
		"Work on GitHub issue #123 in acme/app: implement it with tests.",
		"improve the code",
		"대충 알아서 정리해줘",
		"",
		"fix the failing test in `internal/setup/setup_test.go` without changing the public API",
	}
	for _, text := range inputs {
		a := clarity.Score(text)
		b := clarity.Score(text)
		if a.Score != b.Score || a.Level != b.Level || len(a.Reasons) != len(b.Reasons) {
			t.Errorf("Score(%q) is not deterministic:\n  first:  %+v\n  second: %+v", text, a, b)
		}
	}
}

// TestScore_ClampedTo0_100 verifies Score always returns values in [0, 100].
func TestScore_ClampedTo0_100(t *testing.T) {
	cases := []string{
		// likely high score – all four bonuses
		"implement OAuth2 login in `auth/oauth.go`, all tests must pass without breaking the existing API",
		// likely low score
		"TBD",
		"maybe somehow fix something",
		"",
		// Korean too-short with hedges
		"대충 뭔가",
	}
	for _, text := range cases {
		got := clarity.Score(text)
		if got.Score < 0 || got.Score > 100 {
			t.Errorf("Score(%q).Score = %d, want in [0, 100]", text, got.Score)
		}
	}
}

// TestScore_ReasonsAuditability verifies that the signed integers in Reasons
// sum exactly to Score (for a task where no clamping occurs).
func TestScore_ReasonsAuditability(t *testing.T) {
	// This task triggers all four bonuses and no penalties.
	// 50 + 15 + 15 + 10 + 10 = 100, which equals the clamped score.
	text := "fix the failing test in `internal/setup/setup_test.go` without changing the public API"
	got := clarity.Score(text)
	sum := 0
	for _, reason := range got.Reasons {
		var delta int
		if _, err := fmt.Sscanf(reason, "%d", &delta); err != nil {
			t.Errorf("Reason %q does not start with a signed integer: %v", reason, err)
			continue
		}
		sum += delta
	}
	if sum != got.Score {
		t.Errorf("Reasons deltas sum to %d, but Score = %d\n  reasons: %v",
			sum, got.Score, got.Reasons)
	}
	if len(got.Reasons) == 0 {
		t.Error("Reasons must not be empty")
	}
}
