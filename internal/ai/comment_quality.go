package ai

import (
	"regexp"
	"strings"
)

// PR-1 Comment Quality Hotfix (Omnichannel Sales Copilot track). A doubled
// generation like
//   "Bên em có hỗ trợ sourcing... nhé.Bên em có hỗ trợ sourcing... nhé."
// must NEVER reach Facebook. SanitizeComment dedupes repeated sentences /
// paragraphs and validates basic quality at the queue boundary — independent of
// WHERE the duplication came from (LLM artifact, retry, extension), so it is the
// single safety net for all comment paths.

// maxCommentRunes caps a comment's length; anything longer is rejected as
// low-quality rather than silently truncated mid-sentence.
const maxCommentRunes = 700

// sentenceChunk matches a run of text up to and including its terminal
// punctuation (., !, ?, …), or a trailing fragment with none. Operates on the
// raw (Vietnamese-friendly) string, splitting ONLY at terminal punctuation.
var sentenceChunk = regexp.MustCompile(`[^.!?…]+[.!?…]*`)

func normSentence(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// dedupeRepeated removes duplicate sentences (the "X.X" doubled-generation bug)
// and duplicate paragraphs. A sentence identical (case/whitespace-insensitive) to
// one already emitted is dropped, collapsing "A A", "A.A", and "A\nA".
func dedupeRepeated(text string) string {
	chunks := sentenceChunk.FindAllString(text, -1)
	if len(chunks) <= 1 {
		return strings.TrimSpace(text)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		n := normSentence(c)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, strings.TrimSpace(c))
	}
	return strings.Join(out, " ")
}

// SanitizeComment dedupes repeated sentences/paragraphs and validates quality
// BEFORE a comment is queued. Returns (cleaned, ok, reasonCode); reasonCode is
// "" when ok, otherwise "comment_quality_invalid".
func SanitizeComment(text string) (string, bool, string) {
	cleaned := strings.TrimSpace(dedupeRepeated(text))
	if cleaned == "" {
		return "", false, "comment_quality_invalid"
	}
	if len([]rune(cleaned)) > maxCommentRunes {
		return cleaned, false, "comment_quality_invalid"
	}
	// An anonymous poster must never be addressed by the literal placeholder.
	if strings.Contains(strings.ToLower(cleaned), "anonymous participant") {
		return cleaned, false, "comment_quality_invalid"
	}
	return cleaned, true, ""
}
