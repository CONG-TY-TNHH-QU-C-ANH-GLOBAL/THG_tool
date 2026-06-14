package ai

import "strings"

// Copilot intent — router layer. Maps the normalized prompt + extracted entities
// to an existing action name. Pure classification: no DB / outbound / session
// access; downstream handlers (commentSinglePost, queueLeadOutreach, …) own every
// readiness/coverage/quality/dedup/outbound gate. Behavior is byte-identical to
// the pre-split router — only the keyword sets are named (intent_lexicon.go) and
// the URL/scope checks read off IntentEntities (intent_entities.go).

// deterministicFacebookAction classifies a Copilot prompt into an action name +
// args. Returns ok=false (no match) to let the brain planner handle ambiguity.
func deterministicFacebookAction(prompt string, orgID, accountID int64) (string, map[string]any, bool) {
	folded := foldVietnameseForMatch(strings.ToLower(stripDashboardContext(prompt)))
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

	// Inbox bulk (kept as-is: bulk scope includes bare "lead").
	if containsAnyFolded(folded, lexInboxVerbs) && containsAnyFolded(folded, lexInboxBulkScope) {
		return "inbox_all_leads", args, true
	}

	// §7 direct-link comment on ONE specific post. Checked BEFORE comment_all_leads
	// (so "comment lead này <url>" targets the one post) and excludes crawl verbs
	// (a crawl verb means "scrape this post's comments", handled below).
	if containsAnyFolded(folded, lexCommentVerbs) && !ent.HasCrawlVerb {
		if ent.HasPostURL {
			args["post_url"] = ent.FacebookURLs[0]
			args["nl_prompt"] = stripDashboardContext(prompt)
			return "comment_single_post", args, true
		}
		// No URL but singular phrasing ("comment bài này" / "lead này") and NOT an
		// explicit bulk scope → single-post; the orchestrator asks for the link.
		if len(ent.FacebookURLs) == 0 && ent.HasSpecificScope &&
			!containsAnyFolded(folded, lexBulkScopeStrict) {
			args["nl_prompt"] = stripDashboardContext(prompt)
			return "comment_single_post", args, true
		}
	}

	// Bulk comment requires an explicit bulk scope (no bare singular "lead").
	if containsAnyFolded(folded, lexCommentVerbs) && containsAnyFolded(folded, lexCommentBulkScope) {
		return "comment_all_leads", args, true
	}

	if containsAnyFolded(folded, lexPostingVerbs) {
		args["content"] = strings.TrimSpace(stripDashboardContext(prompt))
		if u := firstFacebookURL(prompt); u != "" {
			args["group_url"] = u
		}
		return "create_job_post", args, true
	}

	if u := firstFacebookURL(prompt); u != "" && containsAnyFolded(folded, lexScrapeVerbs) {
		if isLikelyFacebookPostURL(u) && containsAnyFolded(folded, lexCommentVerbs) {
			args["post_url"] = u
			return "scrape_comments", args, true
		}
		args["url"] = u
		return "scrape_group", args, true
	}

	if firstFacebookURL(prompt) == "" && containsAnyFolded(folded, lexSearchVerbs) {
		query := promptKeywords(prompt)
		if query == "" {
			query = strings.TrimSpace(stripDashboardContext(prompt))
		}
		if query != "" {
			args["query"] = query
			return "search_groups", args, true
		}
	}
	return "", nil, false
}

// RouteDecisionFor returns the SAFE, secret-free observability view of how a
// prompt would route (no cookies/tokens/session/payload). Additive — for
// debug/observability surfaces; re-runs the pure classifier.
func RouteDecisionFor(prompt string) RouteDecision {
	action, _, ok := deterministicFacebookAction(prompt, 0, 0)
	folded := foldVietnameseForMatch(strings.ToLower(stripDashboardContext(prompt)))
	ent := extractIntentEntities(folded, prompt)
	conf, reason := ConfidenceLow, "no deterministic match"
	if ok {
		conf, reason = ConfidenceHigh, "deterministic keyword/URL match"
	}
	return RouteDecision{
		Action:           action,
		Confidence:       conf,
		Reason:           reason,
		URLCount:         len(ent.FacebookURLs),
		HasSpecificScope: ent.HasSpecificScope,
		HasBulkScope:     ent.HasBulkScope,
		HasCrawlVerb:     ent.HasCrawlVerb,
	}
}
