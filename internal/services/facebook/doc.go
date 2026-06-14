// Package facebook is the application/workflow module for the Facebook vertical:
// crawl group/post, import post, plan comment/inbox, post to group/profile. It owns
// FB-specific sequencing and gates, delegating execution to outbound + connectors,
// intelligence to ai, and URL trust to fburl.
//
// Architecture role: SERVICES/FACEBOOK — see MODULE_BOUNDARIES.md (services/facebook).
//
//   - Allowed imports (conceptual): ai, fburl, outbound (via its port), connectors/
//     identities (via ports), leads/crawl accessors, events (publish), models.
//   - Forbidden imports (conceptual): drivers/copilot (the driver that calls it),
//     sibling services (services/taobao, services/1688), internal/server transport,
//     raw execution_attempts/action_ledger writes (coordination owns those).
//
// SCAFFOLD ONLY (Phase A): boundary marker. Facebook workflows currently live in
// cmd/scraper (queueLeadOutreach, commentSinglePost, crawl submission),
// internal/jobhandlers/facebook_crawl, and internal/leadingest. They migrate here in
// roadmap Phase C (boundary) / Phase H (features re-implemented on the outbox).
package facebook
