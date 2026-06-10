package ai

import (
	"regexp"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// Brand-trust contact alignment (PR-1). The Company Identity is the source of truth:
// a comment may cite the grounded website AND the official contact (a Telegram
// handle / Zalo number / plain label), with at most ONE URL. The validator rejects
// ONLY a contact that is not in the Company Identity; a repair layer first tries to
// bring the comment back into the allowlist instead of dropping the lead.

var reTelegramHandle = regexp.MustCompile(`(?i)(?:t\.me/|telegram[:\s]*@?|@)([a-z0-9_]{4,32})`)

// allowedContactURLs returns the normalized URLs a comment MAY contain: the grounded
// website plus the official contact when it is itself a URL (e.g. t.me/handle).
func allowedContactURLs(id models.CompanyIdentity) []string {
	out := []string{}
	if w := id.AllowedURL(); w != "" {
		out = append(out, w)
	}
	if u := reCommentURL.FindString(strings.ToLower(id.OfficialContact)); u != "" {
		out = append(out, normURLForMatch(u))
	}
	return out
}

func urlMatchesAny(u string, allowed []string) bool {
	n := normURLForMatch(u)
	for _, a := range allowed {
		if a != "" && strings.Contains(n, a) {
			return true
		}
	}
	return false
}

// telegramHandle extracts an "@handle" from the official contact (t.me/X, @X,
// "telegram X"); "" when the contact is not a Telegram handle.
func telegramHandle(contact string) string {
	m := reTelegramHandle.FindStringSubmatch(contact)
	if len(m) == 2 {
		return "@" + m[1]
	}
	return ""
}

// RepairCommentContacts brings a comment into the brand-trust allowlist instead of
// rejecting it: it converts a t.me link for the OFFICIAL handle into the @handle (so
// the website stays the only URL) and strips any URL that is NOT the grounded
// website (a non-grounded / invented link). Returns the repaired text + whether
// anything changed. Re-screen the result before trusting it.
func RepairCommentContacts(text string, id models.CompanyIdentity) (string, bool) {
	out := text
	changed := false

	if h := telegramHandle(id.OfficialContact); h != "" {
		name := strings.TrimPrefix(h, "@")
		re := regexp.MustCompile(`(?i)\b(?:https?://)?t\.me/` + regexp.QuoteMeta(name) + `\b`)
		if re.MatchString(out) {
			out = re.ReplaceAllString(out, h)
			changed = true
		}
	}

	web := id.AllowedURL()
	out = reCommentURL.ReplaceAllStringFunc(out, func(u string) string {
		if web != "" && strings.Contains(normURLForMatch(u), web) {
			return u // keep the grounded website
		}
		changed = true
		return "" // drop a non-grounded URL
	})

	if changed {
		out = regexp.MustCompile(`\s{2,}`).ReplaceAllString(out, " ")
		out = strings.TrimSpace(out)
	}
	return out, changed
}
