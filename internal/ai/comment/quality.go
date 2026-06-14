package comment

import (
	"regexp"
	"strings"

	"github.com/thg/scraper/internal/models"
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
// "" when ok, otherwise a SPECIFIC subreason so the operator can see WHY a
// comment failed the gate instead of a single opaque "comment_quality_invalid":
//   - comment_quality_empty       — nothing left after dedupe/trim
//   - comment_quality_too_long    — exceeds the length cap (maxCommentRunes)
//   - comment_quality_placeholder — addresses the literal "anonymous participant"
func SanitizeComment(text string) (string, bool, string) {
	cleaned := strings.TrimSpace(dedupeRepeated(text))
	if cleaned == "" {
		return "", false, "comment_quality_empty"
	}
	if len([]rune(cleaned)) > maxCommentRunes {
		return cleaned, false, "comment_quality_too_long"
	}
	// An anonymous poster must never be addressed by the literal placeholder.
	if strings.Contains(strings.ToLower(cleaned), "anonymous participant") {
		return cleaned, false, "comment_quality_placeholder"
	}
	return cleaned, true, ""
}

var (
	reCommentURL   = regexp.MustCompile(`https?://[^\s)]+|(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+(?:com|vn|net|org|io|co|shop|store|info|biz|me)(?:/[^\s)]*)?`)
	reCommentEmail = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	reCommentPhone = regexp.MustCompile(`(?:\+?84|0)(?:[\s.\-]?\d){8,10}`)
)

// ScreenCommentContacts enforces the CTA/contact policy (PR-3): a comment may cite
// AT MOST ONE URL and it MUST be the grounded company website; any email/phone MUST
// be the grounded official contact. Anything else is a fabricated / non-grounded
// contact and is rejected. Returns (ok, reasonCode):
//   - comment_multiple_urls    — more than one URL
//   - comment_unsupported_contact — a website/email/phone not grounded in identity
func ScreenCommentContacts(text string, identity models.CompanyIdentity) (bool, string) {
	lower := strings.ToLower(text)
	// Strip emails before URL detection so an email's domain is not double-counted
	// as a URL.
	lowerNoEmail := reCommentEmail.ReplaceAllString(lower, " ")
	urls := reCommentURL.FindAllString(lowerNoEmail, -1)
	if len(urls) > 1 {
		return false, "comment_multiple_urls"
	}
	// The single permitted URL may be EITHER the grounded website OR the official
	// contact when that contact is itself a URL (e.g. t.me/handle) — both are in the
	// Company Identity allowlist.
	if len(urls) == 1 {
		if !urlMatchesAny(urls[0], allowedContactURLs(identity)) {
			return false, "comment_unsupported_contact"
		}
	}
	contact := strings.ToLower(identity.OfficialContact)
	for _, m := range reCommentEmail.FindAllString(lower, -1) {
		if contact == "" || !strings.Contains(contact, m) {
			return false, "comment_unsupported_contact"
		}
	}
	contactDigits := digitsOnly(contact)
	for _, m := range reCommentPhone.FindAllString(text, -1) {
		d := digitsOnly(m)
		if len(d) < 9 {
			continue // too short to be a phone number
		}
		if contactDigits == "" || !strings.Contains(contactDigits, d) {
			return false, "comment_unsupported_contact"
		}
	}
	return true, ""
}

func normURLForMatch(u string) string {
	s := strings.ToLower(strings.TrimSpace(u))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")
	return strings.TrimRight(s, "/")
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
