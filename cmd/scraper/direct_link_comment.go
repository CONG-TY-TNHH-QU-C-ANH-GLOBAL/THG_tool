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
func commentSinglePost(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, args map[string]any, notify func(string), intake *directPostIntake) (string, error) {
	res := resolveDirectCommentURL(argString(args, "nl_prompt"), argString(args, "post_url"))
	if res.blocked {
		return res.message, nil
	}
	orgID := argInt64(args, "org_id")
	// P1.3D live-identity account guard: an explicit direct-post comment must run on the
	// account whose LIVE Chrome connector identity is verified. Fail closed (no first-ready
	// fallback) when identity is missing / ambiguous / mismatched — creating NO workflow,
	// NO import, NO outbound. On success it pins args["account_id"] to the resolved account
	// so the whole chain (workflow == import == comment) uses one identity-verified account.
	if msg, blocked := guardDirectPostAccount(db, args); blocked {
		return msg, nil
	}
	postFBID := fburl.ExtractFacebookPostID(res.canonical)
	groupRef := fburl.ExtractGroupRef(res.canonical)
	// STRICT post-lead lookup: exact canonical OR same post_fbid in the SAME group —
	// never a bare post_fbid match (a group permalink id and a global story_fbid can be
	// DIFFERENT posts), so we never comment on the wrong post.
	lead, err := db.Leads().GetPostLeadByDirectPostRef(ctx, orgID, postFBID, res.canonical, groupRef)
	if err != nil {
		return "", err
	}
	if lead == nil {
		// Unknown post → durable intake: import this one post, then the poller queues
		// the comment once the lead exists. NOT scrape_comments, NOT bulk crawl.
		// intake==nil keeps the legacy scan-required copy (defensive; tests without wiring).
		if intake == nil {
			return "Bài viết này chưa có trong hệ thống. Hãy quét/import bài viết trước khi comment.", nil
		}
		return intake.request(ctx, directPostCommentInput{
			OrgID:             orgID,
			RequestedByUserID: argInt64(args, "user_id"),
			AccountID:         argInt64(args, "account_id"),
			UserRole:          argString(args, "user_role"),
			CanonicalPostURL:  res.canonical,
			PostFBID:          postFBID,
			GroupRef:          groupRef,
			Prompt:            argString(args, "nl_prompt"),
		})
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
