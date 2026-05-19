// Package fburl holds pure Facebook URL helpers shared across layers.
//
// Lives in its own package because both internal/leadingest (write-time
// validation + rescue) and internal/store (read-time repair) consume
// them — putting either as the owner would create an import cycle. Pure
// functions only: no IO, no time.Now, no DB.
package fburl

import "strings"

// LooksLikePostURL is true when the URL carries an identifier that
// resolves to a specific Facebook post when opened. Group / page /
// profile shells (e.g. facebook.com/groups/123 with no further path)
// return false — they would route to the feed, not the post.
func LooksLikePostURL(u string) bool {
	if u == "" {
		return false
	}
	return strings.Contains(u, "/posts/") ||
		strings.Contains(u, "/permalink/") ||
		strings.Contains(u, "story_fbid=") ||
		strings.Contains(u, "multi_permalinks=") ||
		strings.Contains(u, "fbid=")
}

// LooksLikeCommentOnlyURL reports whether a URL points at a comment
// with no parent-post context — i.e. cannot serve as a primary post
// link. A URL with both comment_id AND a post marker (the post URL
// with a comment_id query param) is fine; only standalone comment
// links are rejected.
func LooksLikeCommentOnlyURL(u string) bool {
	if u == "" {
		return false
	}
	hasComment := strings.Contains(u, "comment_id=") || strings.Contains(u, "/comment/")
	return hasComment && !LooksLikePostURL(u)
}

// CanonicalPostPermalink builds a stable Facebook post URL from the IDs
// the crawler extracts. Used as the server-side rescue when the
// DOM-scraped URL is a group / profile shell — common when Facebook
// virtualises the permalink anchor until hover.
//
//   groupFBID empty → falls back to the global permalink form.
//   postFBID empty  → returns "" (no rescue possible).
//
// Synthesis uses the /permalink/ URL form (NOT /posts/). The /permalink/
// form is the canonical Facebook navigation path for group posts and
// reliably resolves regardless of which internal id (story_fbid vs
// top_level_post_id) the caller passed in. The /posts/ form historically
// resolved both but post-2026 changes made it reject top_level_post_id —
// causing the "content isn't available" production bug where 7/8 crawled
// leads opened dead pages.
func CanonicalPostPermalink(groupFBID, postFBID string) string {
	postFBID = strings.TrimSpace(postFBID)
	if postFBID == "" {
		return ""
	}
	if g := strings.TrimSpace(groupFBID); g != "" {
		return "https://www.facebook.com/groups/" + g + "/permalink/" + postFBID + "/"
	}
	return "https://www.facebook.com/permalink.php?story_fbid=" + postFBID
}

// ExtractFacebookPostID parses the Facebook-side post id out of a
// permalink. Returns "" when no canonical id is recognisable.
//
// Marker order is load-bearing: /permalink/ FIRST because that path
// always carries the URL-resolvable story_fbid. /posts/ LAST because
// Facebook sometimes renders the FB-internal top_level_post_id there,
// which does NOT resolve as a URL (the "content isn't available"
// production bug — different from story_fbid). When the same URL has
// both forms, the /permalink/ extraction is the one we want.
func ExtractFacebookPostID(u string) string {
	if u == "" {
		return ""
	}
	for _, marker := range []string{"/permalink/", "story_fbid=", "?fbid=", "&fbid=", "/posts/"} {
		i := strings.Index(u, marker)
		if i < 0 {
			continue
		}
		rest := u[i+len(marker):]
		if id := cutAtNonDigit(rest); id != "" {
			return id
		}
	}
	return ""
}

func cutAtNonDigit(s string) string {
	for i, c := range s {
		if c < '0' || c > '9' {
			return s[:i]
		}
	}
	return s
}

// ExtractFacebookEntityID returns a canonical identifier for the Facebook
// entity addressed by u, suitable for cross-checking that two URLs
// reference the SAME post even when they use different URL shapes.
//
// Difference from [ExtractFacebookPostID]: this function also recognises
// the compact "pfbid<token>" form Facebook ships in profile and page
// post permalinks (e.g. /<user>/posts/pfbid02R3qUXY...).
// ExtractFacebookPostID only handles the numeric form because it serves
// URL CONSTRUCTION ([CanonicalPostPermalink] needs the numeric story id).
//
//   - Use [ExtractFacebookEntityID] for IDENTITY COMPARISON
//     (verifier defense-in-depth, dedup keys, replay traces).
//   - Use [ExtractFacebookPostID] for URL CONSTRUCTION
//     (CanonicalPostPermalink synthesis).
//
// Returns "" when u is empty, malformed, or carries no recognisable
// identifier. The empty string is the FAIL CLOSED signal — callers that
// rely on identity comparison MUST treat "" as a non-match against any
// non-empty id, even when both sides of the comparison are "".
func ExtractFacebookEntityID(u string) string {
	if u == "" {
		return ""
	}

	// /watch/<numeric> path form — handle BEFORE the generic pfbid block
	// so /watch/?v=… (covered later) does not collide with /watch/<id>.
	if id := afterMarkerWithBreak(u, "/watch/", isVideoIDByte); id != "" && !strings.HasPrefix(id, "?") {
		return id
	}

	// pfbid<base64ish> form. Marker is the literal "pfbid" prefix and the
	// token body runs until the first non-pfbid character. We DO NOT
	// lowercase: the body is base64-ish (mixed case is significant on
	// Facebook's side) — folding case would create false matches between
	// genuinely different entities.
	if i := strings.Index(u, "pfbid"); i >= 0 {
		// Anchor to a URL boundary so an accidental "pfbid" substring
		// inside a content payload doesn't get treated as an id. We
		// require either the start of the URL fragment OR a preceding
		// path/query separator.
		if i == 0 || isPfbidBoundary(u[i-1]) {
			rest := u[i:]
			end := pfbidTokenEnd(rest)
			if end >= len("pfbid")+8 { // "pfbid" + ≥8 char body — guards against accidental "pfbid" mention
				return rest[:end]
			}
		}
	}

	// Numeric identifiers. Marker order matches [ExtractFacebookPostID]
	// for consistency: /permalink/ first because that path always carries
	// the URL-resolvable story_fbid.
	for _, marker := range []string{"/permalink/", "story_fbid=", "?fbid=", "&fbid=", "/posts/", "?v=", "&v="} {
		i := strings.Index(u, marker)
		if i < 0 {
			continue
		}
		rest := u[i+len(marker):]
		if id := cutAtNonDigit(rest); id != "" {
			return id
		}
	}
	return ""
}

// SameFacebookEntity reports whether two URLs address the same Facebook
// entity for identity-comparison purposes. False when EITHER URL has no
// extractable id ("fail closed"): a caller that cannot independently
// verify both sides must not be allowed to claim a match.
func SameFacebookEntity(a, b string) bool {
	idA := ExtractFacebookEntityID(a)
	idB := ExtractFacebookEntityID(b)
	if idA == "" || idB == "" {
		return false
	}
	return idA == idB
}

// isPfbidBoundary returns true for characters that mark a URL field
// boundary preceding a pfbid token (/, =, &, ?). Keeps the "pfbid"
// prefix from being misread when it happens to appear inside encoded
// content (e.g. tracking params).
func isPfbidBoundary(c byte) bool {
	switch c {
	case '/', '=', '&', '?':
		return true
	}
	return false
}

// pfbidTokenEnd returns the index in s where the pfbid token ends. The
// pfbid body uses [A-Za-z0-9] (base64-without-padding) so any other
// byte (including '/', '?', '&', '_', '-') terminates.
func pfbidTokenEnd(s string) int {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i < len("pfbid") {
			continue
		}
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		return i
	}
	return len(s)
}

// afterMarkerWithBreak extracts the substring starting after marker, up
// to the first byte the supplied predicate rejects. Returns "" when
// marker is absent or no characters were captured.
func afterMarkerWithBreak(u, marker string, accept func(byte) bool) string {
	i := strings.Index(u, marker)
	if i < 0 {
		return ""
	}
	rest := u[i+len(marker):]
	for j := 0; j < len(rest); j++ {
		if !accept(rest[j]) {
			return rest[:j]
		}
	}
	return rest
}

func isVideoIDByte(c byte) bool {
	return c >= '0' && c <= '9'
}
