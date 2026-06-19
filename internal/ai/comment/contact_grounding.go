package comment

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

// hostOf extracts the registrable host from a normURLForMatch-normalized URL:
// the part before the first "/", with any trailing punctuation the URL regex may
// have greedily captured (e.g. "thgfulfill.com," from "...thgfulfill.com, dịch
// vụ") trimmed off. "thgfulfill.com/thg-fulfill" → "thgfulfill.com".
func hostOf(normalized string) string {
	h := normalized
	if i := strings.Index(h, "/"); i >= 0 {
		h = h[:i]
	}
	return strings.TrimRight(h, ".,;:!?)]}\"'…")
}

// urlMatchesAny reports whether u's HOST exactly equals an allowed host. The match
// is host-anchored, NOT a substring: a brand-trust allowlist that used
// strings.Contains let a lookalike survive (thgfulfill.com.evil.com and
// fake-thgfulfill.com both "contain" thgfulfill.com). Only the exact configured
// host matches — www. is already stripped by normURLForMatch, and an intentional
// subdomain is allowed only when it is itself the configured allowlist host.
func urlMatchesAny(u string, allowed []string) bool {
	h := hostOf(normURLForMatch(u))
	if h == "" {
		return false
	}
	for _, a := range allowed {
		if a != "" && h == hostOf(a) {
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

	// PR-6: heal spaced/malformed/non-canonical mentions of the grounded
	// website into its ONE canonical clickable form before the URL pass,
	// so a repairable variant is normalized instead of dropped.
	if id.Website != "" {
		if fixed, c := RepairWebsiteMentions(out, id.Website); c {
			out, changed = fixed, true
		}
	}

	if h := telegramHandle(id.OfficialContact); h != "" {
		name := strings.TrimPrefix(h, "@")
		re := regexp.MustCompile(`(?i)\b(?:https?://)?t\.me/` + regexp.QuoteMeta(name) + `\b`)
		if re.MatchString(out) {
			out = re.ReplaceAllString(out, h)
			changed = true
		}
	}

	// PR-6b: keep AT MOST ONE clickable URL. A comment routinely carries TWO
	// grounded links on the same domain — the bare website AND a service/product
	// deep link (e.g. thgfulfill.com + thgfulfill.com/thg-fulfill). Both pass the
	// "is it grounded" test, so the old strip pass kept both and the re-screen
	// still failed comment_multiple_urls (the dominant comment_all_leads skip).
	// Collapse to a single company website: keep the FIRST grounded URL (a
	// deep/service link is normalized down to the bare canonical website — the
	// only company URL comment policy allows by default), and drop every later
	// URL plus every non-grounded one.
	web := id.AllowedURL()
	canonical := CanonicalWebsite(id.Website)
	allowed := allowedContactURLs(id)
	kept := false
	out = reCommentURL.ReplaceAllStringFunc(out, func(u string) string {
		if !urlMatchesAny(u, allowed) {
			changed = true
			return "" // non-grounded / invented link
		}
		if kept {
			changed = true
			return "" // already have the one permitted URL — collapse the rest
		}
		kept = true
		// A website deep/service link (thgfulfill.com/thg-fulfill) is collapsed
		// down to the bare canonical website; product/service URLs are not
		// allowed in comments unless explicitly enabled by policy.
		if web != "" && canonical != "" && strings.Contains(normURLForMatch(u), web) && u != canonical {
			changed = true
			return canonical
		}
		return u
	})

	if changed {
		out = regexp.MustCompile(`\s{2,}`).ReplaceAllString(out, " ")
		out = strings.TrimSpace(out)
	}
	return out, changed
}

// EnsureWebsite deterministically guarantees a CONFIGURED company website appears
// in the comment EXACTLY ONCE (Sprint-6 follow-up). The prompt asks the model to
// include it, but a model can still omit it; this guard closes that gap WITHOUT
// fabricating — it only ever appends `id.Website` (the grounded, canonical URL),
// never an invented one. No-op when no website is grounded, the text is empty, or
// a grounded website variant is already present (http/www/bare all match by host).
// It appends the single canonical URL, so a comment that had no URL stays within
// the ≤1-URL contact policy. Run it AFTER RepairCommentContacts/ScreenCommentContacts.
func EnsureWebsite(text string, id models.CompanyIdentity) (string, bool) {
	canonical := CanonicalWebsite(id.Website)
	if canonical == "" || strings.TrimSpace(text) == "" {
		return text, false
	}
	// Any URL already present — the website in any variant, OR a grounded contact
	// link such as t.me/handle — is a no-op, so we never push the comment to two
	// URLs (the ≤1-URL contact policy). By this point the screen/repair pass has
	// already stripped non-grounded links, so a surviving URL is grounded.
	if reCommentURL.MatchString(text) {
		return text, false
	}
	return strings.TrimRight(strings.TrimSpace(text), " .") + ". " + canonical, true
}
