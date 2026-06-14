package main

import (
	"context"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/fburl"
	"github.com/thg/scraper/internal/store"
)

// §7 natural-language direct-link comment orchestration (usecase layer).
//
// Layering: URL parsing/normalization is platform (internal/fburl); comment
// validation/repair is neutral intelligence (internal/ai/comment, reached via
// queueLeadOutreach); the eligibility/readiness/coverage/dedup gates are reused
// wholesale from queueLeadOutreach — this flow adds NO new gate and bypasses none.

// directCommentResolution is the pure URL-layer decision: either a blocking
// user message (blocked) or a canonical post URL to proceed with.
type directCommentResolution struct {
	canonical string
	message   string // set only when blocked
	blocked   bool
}

// resolveDirectCommentURL extracts exactly one Facebook post URL from the prompt
// (or the router-provided post_url) and normalizes it to a canonical post ref.
// Pure (no IO) so the §7 URL states are unit-testable.
func resolveDirectCommentURL(prompt, postURLArg string) directCommentResolution {
	urls := fburl.ExtractFacebookURLs(prompt)
	if len(urls) == 0 && postURLArg != "" {
		urls = []string{postURLArg}
	}
	switch {
	case len(urls) == 0:
		return directCommentResolution{message: "Bạn gửi giúp tôi link bài viết Facebook cần comment.", blocked: true}
	case len(urls) > 1:
		return directCommentResolution{message: "Bạn chỉ gửi một link bài viết Facebook cho mỗi lần comment.", blocked: true}
	}
	canonical, ok := fburl.CanonicalizePostURL(urls[0])
	if !ok {
		return directCommentResolution{message: "Link Facebook này chưa được hỗ trợ. Hãy gửi link bài viết hoặc link post trong nhóm/trang.", blocked: true}
	}
	return directCommentResolution{canonical: canonical}
}

// commentSinglePost handles the "comment_single_post" agent action: resolve the
// post URL, find the EXISTING lead (never fabricate post content), then delegate
// to queueLeadOutreach scoped to that one lead so every eligibility/readiness/
// coverage/quality/dedup gate and its status copy are reused unchanged.
func commentSinglePost(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, args map[string]any, notify func(string)) (string, error) {
	res := resolveDirectCommentURL(argString(args, "nl_prompt"), argString(args, "post_url"))
	if res.blocked {
		return res.message, nil
	}
	orgID := argInt64(args, "org_id")
	lead, err := db.Leads().GetLeadByPostRef(ctx, orgID, fburl.ExtractFacebookPostID(res.canonical), res.canonical)
	if err != nil {
		return "", err
	}
	if lead == nil {
		return "Bài viết này chưa có trong hệ thống. Hãy quét/import bài viết trước khi comment.", nil
	}
	// Scope the planner to this one existing lead (lead_id) so it carries real
	// content + coverage history. queueLeadOutreach runs the §5 readiness gate,
	// coverage, quality/repair (internal/ai/comment), dedup and policy, and
	// returns the shared status copy (queued / no-ready-account / coverage / etc.).
	qargs := map[string]any{
		"org_id":    orgID,
		"user_id":   argInt64(args, "user_id"),
		"user_role": argString(args, "user_role"),
		"lead_id":   lead.ID,
		"max_items": int64(1),
	}
	if acc := argInt64(args, "account_id"); acc > 0 {
		qargs["account_id"] = acc
	}
	return queueLeadOutreach(ctx, db, msgGen, "comment", qargs, notify)
}
