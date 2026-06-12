package ai

import (
	"regexp"
	"strings"
)

// Canonical company-website output (SaaS UX Hardening PR-6).
// Product rule — deliberately WITHOUT any policy flag: when the
// workspace has a website configured, every comment cites it as ONE
// canonical clickable URL; when the field is empty, no website is
// mentioned. Spaced / malformed variants (thgfulfill. com,
// thgfulfill com, scheme-less partials) are repaired to the canonical
// form, never emitted.

// CanonicalWebsite normalizes the STORED website value into its
// canonical clickable form: https:// scheme (http upgraded), host
// lowercased exactly as configured (www stays iff the operator stored
// www), no trailing slash, inner whitespace healed ("thgfulfill. com"
// → "thgfulfill.com"). Empty in → empty out.
func CanonicalWebsite(stored string) string {
	s := strings.TrimSpace(stored)
	if s == "" {
		return ""
	}
	// Heal accidental whitespace around dots ("thgfulfill. com/vi").
	s = regexp.MustCompile(`\s*\.\s*`).ReplaceAllString(s, ".")
	s = strings.Join(strings.Fields(s), "") // any remaining inner spaces
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimRight(s, "/")
	if s == "" {
		return ""
	}
	// Lowercase the host segment only; the path keeps its casing.
	host, path := s, ""
	if i := strings.Index(s, "/"); i >= 0 {
		host, path = s[:i], s[i:]
	}
	return "https://" + strings.ToLower(host) + path
}

// websiteBaseName extracts the registrable label used for spaced-domain
// detection: "https://www.thgfulfill.com/vi" → ("thgfulfill", "com").
func websiteBaseName(canonical string) (name, tld string) {
	h := normURLForMatch(canonical)
	if i := strings.Index(h, "/"); i >= 0 {
		h = h[:i]
	}
	parts := strings.Split(h, ".")
	if len(parts) < 2 {
		return "", ""
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

// RepairWebsiteMentions rewrites every variant of the grounded website
// in text to its canonical clickable form: spaced domains
// ("thgfulfill. com", "thgfulfill com"), scheme-less or www-less
// variants, and http:// forms all become id.Website exactly. Returns
// the repaired text + whether anything changed. No grounded website →
// no-op (non-grounded URLs are handled by RepairCommentContacts).
func RepairWebsiteMentions(text, canonicalWebsite string) (string, bool) {
	canonical := strings.TrimSpace(canonicalWebsite)
	if canonical == "" || strings.TrimSpace(text) == "" {
		return text, false
	}
	name, tld := websiteBaseName(canonical)
	if name == "" {
		return text, false
	}
	changed := false
	out := text

	// 1. Spaced / broken domain ("thgfulfill. com/vi", "thgfulfill com").
	// At least ONE whitespace must be involved — a correctly joined
	// domain is left for the variant pass below.
	q := regexp.QuoteMeta
	spaced := regexp.MustCompile(`(?i)\b` + q(name) + `(?:\s+[.]\s*|\s*[.]\s+)` + q(tld) + `(/\S*)?|\b` + q(name) + `\s+` + q(tld) + `\b`)
	if spaced.MatchString(out) {
		out = spaced.ReplaceAllString(out, canonical)
		changed = true
	}

	// 2. Well-formed but non-canonical variants of the SAME site
	//    (http://, missing www, bare domain) → canonical, exactly once
	//    per occurrence.
	canonicalMatch := normURLForMatch(canonical)
	out = reCommentURL.ReplaceAllStringFunc(out, func(u string) string {
		n := normURLForMatch(u)
		if n == "" || !strings.HasPrefix(n+"/", strings.SplitN(canonicalMatch, "/", 2)[0]+"/") {
			return u // different site — not ours to rewrite
		}
		if u == canonical {
			return u
		}
		changed = true
		return canonical
	})

	return out, changed
}
