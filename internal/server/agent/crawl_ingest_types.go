package agent

import "errors"

// Typed request/result shapes + domain error sentinels for the connector crawl-result ingest
// (see crawl_ingest.go for the processor). Kept here so the processor file stays focused on
// flow, not data definitions.

// connectorCrawlItem is one observed post the extension reported. JSON tags mirror the
// extension payload; older extensions that omit PostFBID/GroupFBID/PostedAt leave them empty
// and the server falls back to URL parsing (backward compatible).
type connectorCrawlItem struct {
	ID               string `json:"id"`
	SourceURL        string `json:"source_url"`
	AuthorProfileURL string `json:"author_profile_url"`
	AuthorName       string `json:"author_name"`
	Content          string `json:"content"`
	Reactions        int    `json:"reactions"`
	Comments         int    `json:"comments"`
	Shares           int    `json:"shares"`
	PostFBID         string `json:"post_fbid"`
	GroupFBID        string `json:"group_fbid"`
	PostedAt         string `json:"posted_at"` // RFC3339; empty when crawler can't extract
}

// connectorCrawlResultRequest is the typed connector crawl-result payload (replaces the
// former anonymous inline struct in the handler).
type connectorCrawlResultRequest struct {
	TaskID           string               `json:"task_id"`
	Intent           string               `json:"intent"`
	AccountID        int64                `json:"account_id"`
	IntentID         int64                `json:"intent_id"` // recurring crawl intent id; 0 for one-shot runs
	Status           string               `json:"status"`
	Error            string               `json:"error"`
	ExitReason       string               `json:"exit_reason"`
	ScrollDiag       map[string]any       `json:"scroll_diag"` // forensic: passes / max_articles / scroll_moved_ever / ...
	Keywords         []string             `json:"keywords"`
	MarketSignalGate map[string]any       `json:"market_signal_gate"`
	UserPrompt       string               `json:"user_prompt"`
	Items            []connectorCrawlItem `json:"items"`
}

// connectorCrawlProcessResult is what the handler renders to JSON. Status is "stored" for a
// normal ingest or "failed" when the extension reported a failed crawl (Error is set then).
type connectorCrawlProcessResult struct {
	Status   string
	TaskID   string
	Fetched  int
	Inserted int
	Error    string // set only when Status == "failed"
}

// Domain errors the HTTP edge maps to status codes (the handler owns the status mapping; the
// processor only returns semantic errors). Any other error → 500.
var (
	errCrawlForbiddenAccount = errors.New("account does not belong to this organization")
	errCrawlForbiddenStream  = errors.New("connector does not own this account stream")
)
