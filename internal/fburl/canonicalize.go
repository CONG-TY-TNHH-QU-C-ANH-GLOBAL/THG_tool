package fburl

import (
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

// isFacebookHost reports whether u points at a Facebook-owned host (so a
// non-Facebook URL carrying a "/posts/" segment is not mistaken for a FB post).
func isFacebookHost(u string) bool {
	l := strings.ToLower(u)
	return strings.Contains(l, "facebook.com") || strings.Contains(l, "fb.com") || strings.Contains(l, "fb.watch")
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
