package intent

// Copilot intent/routing contract types (Architecture-first refactor — see
// specs/COPILOT_INTENT_ROUTING_ARCHITECTURE.md). These give the routing pipeline
// an explicit shape — raw text → normalize → entities → classify → route — so
// future NLU (typo-tolerant / multilingual) has a clean home. Additive: the
// existing DeterministicFacebookAction contract (string, args, bool) is kept.

// Confidence is the classifier's confidence in a routing decision. Reserved for
// the future NLU layer; the current deterministic classifier emits High on a
// keyword/URL match and Low on no match.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// IntentEntities is the normalized, platform-aware feature set extracted from the
// prompt BEFORE classification. Pure data — no DB/outbound/session access.
type IntentEntities struct {
	FacebookURLs     []string // host-anchored FB URLs (via internal/fburl)
	HasPostURL       bool     // first FB URL looks like a commentable/crawlable post
	HasGroupURL      bool     // an FB URL is present but not a specific post (group/page/profile shell)
	HasSpecificScope bool     // singular phrasing: "bài này" / "post này" / "lead này"
	// Bulk scope is branch-specific: the comment-bulk and inbox-bulk lexicons
	// differ (inbox bulk still accepts a bare "lead"). Keep both so observability
	// matches the branch that actually routed; HasBulkScope is their OR.
	HasCommentBulkScope bool // "leads" / "các lead" / "tất cả" / "tệp khách" …
	HasInboxBulkScope   bool // comment-bulk set PLUS a bare singular "lead"
	HasBulkScope        bool // HasCommentBulkScope || HasInboxBulkScope
	HasCrawlVerb        bool // cào / crawl / scrape / quét
}

// IntentDecision is the classifier output: an action name + why. Entities are
// carried for observability and downstream argument building.
type IntentDecision struct {
	Action     string
	Confidence Confidence
	Reason     string
	Entities   IntentEntities
}

// RouteDecision is the safe, secret-free observability view of a routing
// decision (no cookies/tokens/session/payload). Build it with routeDecision().
type RouteDecision struct {
	Action              string     `json:"action"`
	Confidence          Confidence `json:"confidence"`
	Reason              string     `json:"reason"`
	URLCount            int        `json:"url_count"`
	HasSpecificScope    bool       `json:"has_specific_scope"`
	HasBulkScope        bool       `json:"has_bulk_scope"`
	HasCommentBulkScope bool       `json:"has_comment_bulk_scope"`
	HasInboxBulkScope   bool       `json:"has_inbox_bulk_scope"`
	HasCrawlVerb        bool       `json:"has_crawl_verb"`
}
