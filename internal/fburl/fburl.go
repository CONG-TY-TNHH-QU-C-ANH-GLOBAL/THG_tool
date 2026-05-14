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
func CanonicalPostPermalink(groupFBID, postFBID string) string {
	postFBID = strings.TrimSpace(postFBID)
	if postFBID == "" {
		return ""
	}
	if g := strings.TrimSpace(groupFBID); g != "" {
		return "https://www.facebook.com/groups/" + g + "/posts/" + postFBID + "/"
	}
	return "https://www.facebook.com/permalink.php?story_fbid=" + postFBID
}

// ExtractFacebookPostID parses the Facebook-side post id out of a
// permalink. Returns "" when no canonical id is recognisable. Order
// of markers matters: more specific markers first so story_fbid wins
// over the bare fbid= patterns.
func ExtractFacebookPostID(u string) string {
	if u == "" {
		return ""
	}
	for _, marker := range []string{"/posts/", "/permalink/", "story_fbid=", "?fbid=", "&fbid="} {
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
