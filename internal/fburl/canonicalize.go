package fburl

import (
	"net/url"
	"regexp"
	"strings"
)

var reAnyURL = regexp.MustCompile(`https?://[^\s]+`)

// ExtractFacebookURLs returns every Facebook URL in text, with trailing
// sentence punctuation trimmed. Platform layer owns FB URL recognition so the
// usecase orchestrator can count/validate links without embedding FB knowledge.
func ExtractFacebookURLs(text string) []string {
	var out []string
	for _, raw := range reAnyURL.FindAllString(text, -1) {
		u := strings.TrimRight(raw, ".,);]")
		if isFacebookHost(u) {
			out = append(out, u)
		}
	}
	return out
}

// CanonicalizePostURL normalizes a RAW Facebook post URL — mobile hosts
// (m./mbasic.facebook.com), tracking params, and the various post forms
// (/groups/{g}/posts/{p}, /permalink/, permalink.php?story_fbid=,
// /{page}/posts/{p}, pfbid profile/page posts) — into a stable canonical post
// reference. ok=false when the URL is NOT a commentable post (a group/page/
// profile shell that routes to a feed, or a standalone comment link).
//
// Pure platform logic: it knows Facebook URL shapes so the neutral comment
// intelligence (internal/ai/comment) never has to.
func CanonicalizePostURL(raw string) (string, bool) {
	u := strings.TrimSpace(raw)
	if u == "" || !isFacebookHost(u) {
		return "", false
	}
	if LooksLikeCommentOnlyURL(u) || !LooksLikePostURL(u) {
		return "", false
	}
	// Numeric story id → rebuild the canonical permalink (group-aware). This is
	// the dominant case (group/page posts) and yields a stable, resolvable URL.
	if postID := ExtractFacebookPostID(u); postID != "" {
		return CanonicalPostPermalink(extractGroupFBID(u), postID), true
	}
	// pfbid (profile/page) form carries no numeric story id to rebuild from; the
	// id lives in the path, so strip the mobile host + tracking query and keep it.
	if ExtractFacebookEntityID(u) != "" {
		return cleanPathPostURL(u), true
	}
	return "", false
}

// IsFacebookURL reports whether raw is a URL on a genuine Facebook host. The
// single, host-anchored source of truth for "is this a Facebook URL?" — callers
// across the codebase must use it instead of substring-matching the host, which
// accepts lookalikes (facebook.com.evil.com, fb.com.evil.com, ...).
func IsFacebookURL(raw string) bool { return isFacebookHost(raw) }

var fbBaseHosts = []string{"facebook.com", "fb.com", "fb.watch"}

// isFacebookHost reports whether u's HOST is a Facebook-owned host. The match is
// host-anchored via net/url (exact base or a real subdomain), NOT a substring —
// so lookalikes are rejected: facebook.com.evil.com, notfacebook.com,
// fake-facebook.com, fb.com.evil.com, a userinfo trick (facebook.com@evil.com),
// and facebook.com appearing only in a query/path of a non-Facebook host.
func isFacebookHost(raw string) bool {
	h := hostOfURL(raw)
	if h == "" {
		return false
	}
	for _, base := range fbBaseHosts {
		if h == base || strings.HasSuffix(h, "."+base) {
			return true
		}
	}
	return false
}

// hostOfURL returns the lowercased hostname of raw (port + userinfo stripped),
// adding a scheme when absent so a bare "www.facebook.com/..." still parses.
func hostOfURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// extractGroupFBID returns the numeric group id from a /groups/{id}/ path, or "".
func extractGroupFBID(u string) string {
	i := strings.Index(u, "/groups/")
	if i < 0 {
		return ""
	}
	return cutAtNonDigit(u[i+len("/groups/"):])
}

// cleanPathPostURL normalizes the mobile host to www and drops the query/tracking
// params for a path-based (pfbid) post URL whose id lives in the path.
func cleanPathPostURL(u string) string {
	u = strings.Replace(u, "//m.facebook.com", "//www.facebook.com", 1)
	u = strings.Replace(u, "//mbasic.facebook.com", "//www.facebook.com", 1)
	if i := strings.IndexByte(u, '?'); i >= 0 {
		u = u[:i]
	}
	return strings.TrimRight(u, "/")
}
