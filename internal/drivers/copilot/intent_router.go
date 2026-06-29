package copilot

import (
	"strings"

	"github.com/thg/scraper/internal/drivers/copilot/promptprep"
	"github.com/thg/scraper/internal/drivers/copilot/textnorm"
)

// Copilot intent — router layer. Maps the normalized prompt + extracted entities
// to an existing action name. Pure classification: no DB / outbound / session
// access; downstream handlers (commentSinglePost, queueLeadOutreach, …) own every
// readiness/coverage/quality/dedup/outbound gate. Behavior is byte-identical to
// the pre-split router — only the keyword sets are named (intent_lexicon.go) and
// the URL/scope checks read off IntentEntities (intent_entities.go).

// deterministicFacebookAction classifies a Copilot prompt into an action name +
// args. Returns ok=false (no match) to let the brain planner handle ambiguity. The
// classification ladder is a fixed-order sequence of classifyX helpers (each returns
// ok=true on its match) — extracted from one big function to keep each branch (and
// the dispatch) under the cognitive-complexity threshold; order + behavior unchanged.
func deterministicFacebookAction(prompt string, orgID, accountID int64) (string, map[string]any, bool) {
	folded := textnorm.Fold(strings.ToLower(promptprep.StripDashboardContext(prompt)))
	ent := extractIntentEntities(folded, prompt)
	args := map[string]any{}
	if orgID > 0 {
		args["org_id"] = orgID
	}
	if accountID > 0 {
		args["account_id"] = accountID
	}
	if maxItems := extractMaxItemsFromPrompt(prompt); maxItems > 0 {
		args["max_items"] = maxItems
	}

	if a, ok := classifyInboxBulk(folded); ok {
		return a, args, true
	}
	if a, ok := classifyCommentSingle(folded, ent, prompt, args); ok {
		return a, args, true
	}
	if a, ok := classifyCommentBulk(folded); ok {
		return a, args, true
	}
	if a, ok := classifyPostingAction(folded, prompt, args); ok {
		return a, args, true
	}
	if a, ok := classifyScrape(folded, prompt, args); ok {
		return a, args, true
	}
	if a, ok := classifySearch(folded, prompt, args); ok {
		return a, args, true
	}
	return "", nil, false
}

// classifyInboxBulk — inbox bulk (bulk scope includes a bare "lead").
func classifyInboxBulk(folded string) (string, bool) {
	if textnorm.ContainsAny(folded, lexInboxVerbs) && textnorm.ContainsAny(folded, lexInboxBulkScope) {
		return "inbox_all_leads", true
	}
	return "", false
}

// classifyCommentSingle — §7 direct-link comment on ONE specific post. Checked BEFORE
// comment_all_leads (so "comment lead này <url>" targets the one post) and excludes
// crawl verbs (a crawl verb means "scrape this post's comments", handled later).
func classifyCommentSingle(folded string, ent IntentEntities, prompt string, args map[string]any) (string, bool) {
	if !textnorm.ContainsAny(folded, lexCommentVerbs) || ent.HasCrawlVerb {
		return "", false
	}
	if ent.HasPostURL {
		args["post_url"] = ent.FacebookURLs[0]
		args["nl_prompt"] = promptprep.StripDashboardContext(prompt)
		return "comment_single_post", true
	}
	// No URL but singular phrasing ("comment bài này" / "lead này") and NOT an
	// explicit bulk scope → single-post; the orchestrator asks for the link.
	if len(ent.FacebookURLs) == 0 && ent.HasSpecificScope &&
		!textnorm.ContainsAny(folded, lexBulkScopeStrict) {
		args["nl_prompt"] = promptprep.StripDashboardContext(prompt)
		return "comment_single_post", true
	}
	return "", false
}

// classifyCommentBulk — bulk comment requires an explicit bulk scope (no bare "lead").
func classifyCommentBulk(folded string) (string, bool) {
	if textnorm.ContainsAny(folded, lexCommentVerbs) && textnorm.ContainsAny(folded, lexCommentBulkScope) {
		return "comment_all_leads", true
	}
	return "", false
}

// classifyPostingAction — create a Facebook post (optionally pinned to a group URL).
func classifyPostingAction(folded, prompt string, args map[string]any) (string, bool) {
	if !textnorm.ContainsAny(folded, lexPostingVerbs) {
		return "", false
	}
	args["content"] = strings.TrimSpace(promptprep.StripDashboardContext(prompt))
	if u := firstFacebookURL(prompt); u != "" {
		args["group_url"] = u
	}
	return "create_job_post", true
}

// classifyScrape — a URL + a scrape verb: scrape_comments for a post (with a comment
// verb), else scrape_group.
func classifyScrape(folded, prompt string, args map[string]any) (string, bool) {
	u := firstFacebookURL(prompt)
	if u == "" || !textnorm.ContainsAny(folded, lexScrapeVerbs) {
		return "", false
	}
	if isLikelyFacebookPostURL(u) && textnorm.ContainsAny(folded, lexCommentVerbs) {
		args["post_url"] = u
		return "scrape_comments", true
	}
	args["url"] = u
	return "scrape_group", true
}

// classifySearch — a search verb with no URL: search groups by the prompt keywords.
func classifySearch(folded, prompt string, args map[string]any) (string, bool) {
	if firstFacebookURL(prompt) != "" || !textnorm.ContainsAny(folded, lexSearchVerbs) {
		return "", false
	}
	query := promptKeywords(prompt)
	if query == "" {
		query = strings.TrimSpace(promptprep.StripDashboardContext(prompt))
	}
	if query != "" {
		args["query"] = query
		return "search_groups", true
	}
	return "", false
}

// promptIsDirectPostComment reports whether the prompt is an unambiguous
// "comment on THIS specific post" instruction that must reach the deterministic
// comment_single_post route BEFORE the brain planner. Without this gate the brain
// handles "comment bài này <url>" first and returns generic Copilot text,
// pre-empting the route — the production "generic response" bug this fixes.
//
// True only when ALL hold (returning false is the SAFE default — anything
// ambiguous falls through to the brain, mirroring promptIsSelfSufficient):
//
//  1. A valid Facebook POST URL is present (host-anchored via fburl; a group/
//     page shell or lookalike host does not set HasPostURL, so it won't qualify).
//  2. A comment verb (comment / bình luận) is present.
//  3. NO crawl verb (cào/crawl/scrape/quét) — a crawl verb means "scrape this
//     post's comments" (scrape_comments), not "post a comment".
//  4. NOT a bulk comment scope (leads / các lead / tất cả / tệp khách / all) —
//     bulk stays comment_all_leads.
func promptIsDirectPostComment(prompt string) bool {
	folded := textnorm.Fold(strings.ToLower(promptprep.StripDashboardContext(prompt)))
	ent := extractIntentEntities(folded, prompt)
	if !ent.HasPostURL {
		return false
	}
	if !textnorm.ContainsAny(folded, lexCommentVerbs) {
		return false
	}
	if ent.HasCrawlVerb {
		return false
	}
	if textnorm.ContainsAny(folded, lexCommentBulkScope) {
		return false
	}
	return true
}

// RouteDecisionFor returns the SAFE, secret-free observability view of how a
// prompt would route (no cookies/tokens/session/payload). Additive — for
// debug/observability surfaces; re-runs the pure classifier.
func RouteDecisionFor(prompt string) RouteDecision {
	action, _, ok := deterministicFacebookAction(prompt, 0, 0)
	folded := textnorm.Fold(strings.ToLower(promptprep.StripDashboardContext(prompt)))
	ent := extractIntentEntities(folded, prompt)
	conf, reason := ConfidenceLow, "no deterministic match"
	if ok {
		conf, reason = ConfidenceHigh, "deterministic keyword/URL match"
	}
	return RouteDecision{
		Action:              action,
		Confidence:          conf,
		Reason:              reason,
		URLCount:            len(ent.FacebookURLs),
		HasSpecificScope:    ent.HasSpecificScope,
		HasBulkScope:        ent.HasBulkScope,
		HasCommentBulkScope: ent.HasCommentBulkScope,
		HasInboxBulkScope:   ent.HasInboxBulkScope,
		HasCrawlVerb:        ent.HasCrawlVerb,
	}
}
