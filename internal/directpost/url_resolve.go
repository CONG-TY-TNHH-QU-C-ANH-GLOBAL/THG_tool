package directpost

import "github.com/thg/scraper/internal/fburl"

// §7 direct-link comment URL resolution. Sibling to the identity/context/content
// invariants in this leaf: the pure URL-layer decision a direct-post comment makes
// before any IO — extract exactly ONE Facebook post URL from the prompt (or the
// router-provided post_url) and normalize it to a canonical post ref. The cmd
// orchestration (commentSinglePost) owns the workflow/import/outbound that follows.

// CommentURLResolution is the pure URL-layer decision: either a blocking user
// message (Blocked) or a canonical post URL to proceed with.
type CommentURLResolution struct {
	Canonical string
	Message   string // set only when Blocked
	Blocked   bool
}

// ResolveCommentURL extracts exactly one Facebook post URL from the prompt (or the
// router-provided postURLArg) and normalizes it to a canonical post ref. Pure (no IO)
// so the §7 URL states are unit-testable.
func ResolveCommentURL(prompt, postURLArg string) CommentURLResolution {
	urls := fburl.ExtractFacebookURLs(prompt)
	if len(urls) == 0 && postURLArg != "" {
		urls = []string{postURLArg}
	}
	switch {
	case len(urls) == 0:
		return CommentURLResolution{Message: "Bạn gửi giúp tôi link bài viết Facebook cần comment.", Blocked: true}
	case len(urls) > 1:
		return CommentURLResolution{Message: "Bạn chỉ gửi một link bài viết Facebook cho mỗi lần comment.", Blocked: true}
	}
	canonical, ok := fburl.CanonicalizePostURL(urls[0])
	if !ok {
		return CommentURLResolution{Message: "Link Facebook này chưa được hỗ trợ. Hãy gửi link bài viết hoặc link post trong nhóm/trang.", Blocked: true}
	}
	return CommentURLResolution{Canonical: canonical}
}
