// Package crawlcampaign holds the pure domain language and decision logic for
// the Facebook multi-group fresh-lead crawl
// (specs/facebook/MULTI_GROUP_FRESH_LEAD_CRAWL_SPEC.md): run statuses, typed exit
// / freshness reason codes, the run fence, the canonical server-side timestamp
// DTO, and the pure freshness policy.
//
// It is store-, transport-, and clock-free: every decision takes its inputs
// (including the authoritative server time) as arguments. No production
// entrypoint wires it yet — PR-M3A defines the contracts and policy only.
package crawlcampaign
