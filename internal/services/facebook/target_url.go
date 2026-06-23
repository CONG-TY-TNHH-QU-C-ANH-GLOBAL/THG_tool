package facebook

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// Facebook outbound target-URL resolution (Phase C/I): the FB-specific mapping from a
// lead + msgType to the canonical URL the outbound queue should hit. Moved verbatim out of
// cmd/scraper so the neutral outbound orchestrator delegates FB URL knowledge here. Pure:
// imports only models + stdlib.

// ResolveOutboundTargetURL maps a lead + msgType to the canonical target URL
// the outbound queue should hit. Returns ("", skipReason) when the lead is
// not actionable. Branches on SourceType explicitly so unknown values cannot
// silently fall through to SourceURL (per feedback_deterministic_boundaries).
//
// Routing contract (see models.Lead):
//   - SourceURL is ALWAYS the parent post URL for SourceType in
//     {post, comment, prompt_target}.
//   - SecondaryURL is the comment URL (reply-to-comment, future feature).
//   - For inbox msgType, target is the participant's AuthorURL — SourceURL
//     is not consulted.
//
// PostFBID fallback: if SourceURL is a transient form (photo viewer, share
// shim) that IsCommentableFacebookPostURL rejects but post_fbid is present
// with a known group, reconstruct the canonical /groups/<g>/posts/<p>/ URL.
func ResolveOutboundTargetURL(lead models.Lead, msgType string) (string, string) {
	if msgType == "inbox" {
		if t := strings.TrimSpace(lead.AuthorURL); t != "" {
			return t, ""
		}
		return "", "missing_target"
	}
	switch strings.ToLower(strings.TrimSpace(lead.SourceType)) {
	case "", "post", "comment", "prompt_target":
		target := strings.TrimSpace(lead.SourceURL)
		if msgType == "comment" && !IsCommentableFacebookPostURL(target) {
			if rebuilt := CanonicalGroupPostURLFromFBIDs(lead.GroupFBID, lead.PostFBID); rebuilt != "" {
				return rebuilt, ""
			}
			return "", "missing_post_permalink"
		}
		if target == "" {
			return "", "missing_target"
		}
		return target, ""
	default:
		return "", "unrouted_source_type"
	}
}

// CanonicalGroupPostURLFromFBIDs reconstructs the canonical group-post URL
// from the routing contract's group_fbid + post_fbid. Returns "" if either
// is missing. Used only as a fallback when SourceURL fails the commentable
// check — photo viewer / share shim / story redirect forms still carry the
// real post_fbid via the crawler's URL repair path.
func CanonicalGroupPostURLFromFBIDs(groupFBID, postFBID string) string {
	g := strings.TrimSpace(groupFBID)
	p := strings.TrimSpace(postFBID)
	if g == "" || p == "" {
		return ""
	}
	return fmt.Sprintf("https://www.facebook.com/groups/%s/posts/%s/", g, p)
}

// IsCommentableFacebookPostURL reports whether raw is a Facebook URL that addresses a
// specific commentable post/permalink (not a group/profile home or external link).
func IsCommentableFacebookPostURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	if host != "fb.watch" && !strings.HasSuffix(host, ".fb.watch") &&
		host != "facebook.com" && !strings.HasSuffix(host, ".facebook.com") {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(u.EscapedPath()))
	if (host == "fb.watch" || strings.HasSuffix(host, ".fb.watch")) && strings.Trim(path, "/") != "" {
		return true
	}
	query := u.Query()
	if query.Get("story_fbid") != "" || query.Get("multi_permalinks") != "" {
		return true
	}
	if strings.Contains(path, "/posts/") ||
		strings.Contains(path, "/permalink/") ||
		strings.Contains(path, "/videos/") ||
		strings.Contains(path, "/reel/") ||
		strings.Contains(path, "/watch/") ||
		strings.Contains(path, "/share/") {
		return true
	}
	if strings.HasSuffix(path, "/photo.php") && query.Get("fbid") != "" {
		return true
	}
	return false
}
