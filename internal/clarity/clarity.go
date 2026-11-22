// Package clarity provides a heuristic score (0–100) that ranks how clearly
// a one-line task instruction communicates its requirement.
//
// # Rubric
//
// Scoring starts from [BaseScore] (50). Each signal adjusts it:
//
//	Constant                  Points  Trigger
//	─────────────────────────────────────────────────────────────────────
//	BaseScore                  +50    always
//	ConcreteTargetBonus        +15    names a file path, backtick identifier,
//	                                  #N issue ref, URL, or owner/repo path
//	VerifiableOutcomeBonus     +15    contains outcome-indicating words
//	                                  (test/pass/build/PR/should/must/returns…)
//	ScopeBoundBonus            +10    contains a scope-limiting expression
//	                                  (only/without/except/at most/don't…)
//	ActionVerbBonus            +10    leading action verb for English text
//	                                  (add/fix/implement/refactor…) or any
//	                                  action verb anywhere in Korean text
//	TooShortPenalty            -20    < 4 words (non-CJK) or < 12 runes (CJK)
//	HedgePenalty               -15    hedge or vagueness word detected
//	                                  (maybe/somehow/TBD/알아서/대충…)
//	QuestionPenalty            -15    text contains "?"
//	RunOnPenalty               -10    > 600 runes with no concrete target
//
// The Level is "clear" (≥ 70), "vague" (40–69), or "unclear" (< 40).
// Every adjustment is recorded in [Result.Reasons], making the score auditable.
//
// Note: this is a heuristic for ranking queued tasks; it is not a measurement
// of truth. The same input always produces the same output (deterministic).
// No network calls or LLM inference are used.
package clarity

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Weight constants – every adjustment applied by [Score] uses one of these.
const (
	// BaseScore is the starting point before any signals are applied.
	BaseScore = 50

	// ConcreteTargetBonus is added when the text names a concrete artifact
	// (file path, backtick identifier, #N issue ref, URL, or owner/repo).
	ConcreteTargetBonus = 15

	// VerifiableOutcomeBonus is added when the text contains words indicating
	// a checkable outcome (test, pass, build, lint, should, must, PR, …).
	VerifiableOutcomeBonus = 15

	// ScopeBoundBonus is added when the text includes a scope-limiting phrase
	// (only, without, except, at most, don't, …).
	ScopeBoundBonus = 10

	// ActionVerbBonus is added when the text leads with a recognised action
	// verb (English) or contains one anywhere (Korean).
	ActionVerbBonus = 10

	// TooShortPenalty is subtracted when the text is too brief to be
	// actionable: < 4 words for non-CJK text, < 12 runes for CJK text.
	TooShortPenalty = -20

	// HedgePenalty is subtracted when hedge or vagueness words are present
	// (maybe, somehow, TBD, 알아서, 대충, …).
	HedgePenalty = -15

	// QuestionPenalty is subtracted when the text contains "?", indicating
	// the user is asking rather than instructing.
	QuestionPenalty = -15

	// RunOnPenalty is subtracted when the text exceeds 600 runes without
	// naming any concrete target.
	RunOnPenalty = -10
)

// Result is the output of [Score].
type Result struct {
	// Score is a heuristic 0–100 rating of requirement clarity.
	Score int
	// Level is "clear" (≥ 70), "vague" (40–69), or "unclear" (< 40).
	Level string
	// Reasons lists every adjustment applied, each prefixed with its signed
	// point value (e.g. "+15 names a concrete target") so the score is
	// fully auditable.
	Reasons []string
}

// precompiled patterns for concrete-target detection.
var (
	reBacktick = regexp.MustCompile("`[^`]+`")
	reIssueRef = regexp.MustCompile(`#\d+`)
	reURL      = regexp.MustCompile(`https?://`)
	// reSlashPath matches owner/repo or dir/file: at least two chars each side.
	reSlashPath = regexp.MustCompile(`[A-Za-z0-9_.][A-Za-z0-9_.-]+/[A-Za-z0-9_.][A-Za-z0-9_.-]+`)
)

// hasCJK reports whether s contains any CJK (Han, Hangul, Hiragana, Katakana) rune.
func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hangul, r) ||
			unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			return true
		}
	}
	return false
}

// containsAny reports whether lower contains any of the given substrings.
func containsAny(lower string, subs ...string) bool {
	for _, s := range subs {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// levelOf maps a clamped score to its level string.
func levelOf(score int) string {
	switch {
	case score >= 70:
		return "clear"
	case score >= 40:
		return "vague"
	default:
		return "unclear"
	}
}

// clamp restricts n to [0, 100].
func clamp(n int) int {
	if n > 100 {
		return 100
	}
	if n < 0 {
		return 0
	}
	return n
}

// Score rates one task line. Deterministic: same input → same output.
// See package-level documentation for the full rubric.
func Score(text string) Result {
	lower := strings.ToLower(text)
	runes := []rune(text)
	score := BaseScore
	r := Result{Reasons: []string{fmt.Sprintf("+%d base score", BaseScore)}}

	// ── Concrete target ──────────────────────────────────────────────────
	hasTarget := reBacktick.MatchString(text) ||
		reIssueRef.MatchString(text) ||
		reURL.MatchString(text) ||
		reSlashPath.MatchString(text)
	if hasTarget {
		score += ConcreteTargetBonus
		r.Reasons = append(r.Reasons,
			fmt.Sprintf("+%d names a concrete target (file path, identifier, issue ref, URL, or owner/repo)", ConcreteTargetBonus))
	}

	// ── Verifiable outcome ───────────────────────────────────────────────
	outcomeEN := []string{
		"test", "pass", "build", "lint", "returns", "should", "must", "so that",
		"pull request", " pr ", " pr.", " pr,",
	}
	outcomeKO := []string{"테스트", "통과", "해야", "되도록"}
	hasPREnd := strings.HasSuffix(lower, " pr") || strings.HasSuffix(lower, "\npr")
	hasOutcome := containsAny(lower, outcomeEN...) || containsAny(lower, outcomeKO...) || hasPREnd
	if hasOutcome {
		score += VerifiableOutcomeBonus
		r.Reasons = append(r.Reasons,
			fmt.Sprintf("+%d states a verifiable outcome", VerifiableOutcomeBonus))
	}

	// ── Scope bound ──────────────────────────────────────────────────────
	scopeEN := []string{"only", "do not", "without", "except", "at most", "don't", "doesn't"}
	scopeKO := []string{"하지 말", "제외", "이내", "만을", "에만", "만으로", "하지 않"}
	hasScope := containsAny(lower, scopeEN...) || containsAny(lower, scopeKO...)
	if hasScope {
		score += ScopeBoundBonus
		r.Reasons = append(r.Reasons,
			fmt.Sprintf("+%d bounds the scope", ScopeBoundBonus))
	}

	// ── Action verb ──────────────────────────────────────────────────────
	verbsEN := []string{
		"add", "fix", "implement", "refactor", "remove", "rename", "write",
		"update", "create", "delete", "run", "check", "test", "verify", "ensure",
		"move", "change", "replace", "migrate", "deploy", "revert", "bump",
		"enable", "disable", "extract", "merge", "split", "install", "upgrade",
		"downgrade",
	}
	verbsKO := []string{
		"추가", "수정", "구현", "리팩터", "삭제", "이름 변경", "작성", "업데이트",
		"생성", "제거", "변경", "이동", "교체", "마이그레이션", "배포", "설치",
	}
	hasVerb := false
	if hasCJK(text) {
		// Korean is verb-final; check anywhere in the text.
		hasVerb = containsAny(lower, verbsKO...)
	} else {
		// English: check the first word only.
		if fields := strings.Fields(lower); len(fields) > 0 {
			first := strings.TrimRight(fields[0], ".,;:!?")
			for _, v := range verbsEN {
				if first == v {
					hasVerb = true
					break
				}
			}
		}
	}
	if hasVerb {
		score += ActionVerbBonus
		r.Reasons = append(r.Reasons,
			fmt.Sprintf("+%d has an action verb", ActionVerbBonus))
	}

	// ── Too short ────────────────────────────────────────────────────────
	tooShort := false
	if hasCJK(text) {
		if len(runes) < 12 {
			tooShort = true
		}
	} else {
		if len(strings.Fields(text)) < 4 {
			tooShort = true
		}
	}
	if tooShort {
		score += TooShortPenalty // constant is negative
		r.Reasons = append(r.Reasons,
			fmt.Sprintf("%d too short (< 4 words, or < 12 runes for CJK)", TooShortPenalty))
	}

	// ── Hedges and vagueness ─────────────────────────────────────────────
	// alwaysHedge: penalised regardless of whether a concrete target is present.
	alwaysHedgeEN := []string{
		"maybe", "somehow", " etc", "tbd", "or whatever", "make it better", "something",
	}
	alwaysHedgeKO := []string{"알아서", "대충", "적당히", "그냥", "뭔가", "어떻게든", "좀 더 좋게"}
	// vagueVerbs: penalised only when no concrete target names a specific subject.
	vagueVerbsEN := []string{"improve", "optimize", "clean up", "enhance", "cleanup"}
	vagueVerbsKO := []string{"개선", "최적화"}
	hasHedge := containsAny(lower, alwaysHedgeEN...) ||
		containsAny(lower, alwaysHedgeKO...) ||
		(!hasTarget && (containsAny(lower, vagueVerbsEN...) || containsAny(lower, vagueVerbsKO...)))
	if hasHedge {
		score += HedgePenalty // constant is negative
		r.Reasons = append(r.Reasons,
			fmt.Sprintf("%d hedge or vagueness words detected", HedgePenalty))
	}

	// ── Question mark ────────────────────────────────────────────────────
	if strings.Contains(text, "?") {
		score += QuestionPenalty // constant is negative
		r.Reasons = append(r.Reasons,
			fmt.Sprintf("%d text contains a question mark", QuestionPenalty))
	}

	// ── Very long run-on without target ──────────────────────────────────
	if len(runes) > 600 && !hasTarget {
		score += RunOnPenalty // constant is negative
		r.Reasons = append(r.Reasons,
			fmt.Sprintf("%d very long run-on (> 600 runes) without any concrete target", RunOnPenalty))
	}

	// ── Clamp and finalise ───────────────────────────────────────────────
	score = clamp(score)
	r.Score = score
	r.Level = levelOf(score)
	return r
}
