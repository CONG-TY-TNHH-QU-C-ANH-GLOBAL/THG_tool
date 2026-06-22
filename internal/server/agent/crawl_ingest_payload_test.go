package agent

import (
	"encoding/json"
	"testing"
)

// rawCrawlResultPayload is the connector crawl-result JSON exactly as the Chrome
// extension posts it (header + scroll_diag + keywords + market_signal_gate + one
// item + an unknown future field).
const rawCrawlResultPayload = `{
	"task_id": "task-77",
	"intent": "facebook_crawl",
	"account_id": 4242,
	"intent_id": 9,
	"status": "completed",
	"error": "",
	"exit_reason": "end_of_feed",
	"scroll_diag": {"passes": 6, "max_articles_seen": 14, "scroll_moved_ever": true},
	"keywords": ["pod", "áo thun"],
	"market_signal_gate": {"min_reactions": 3},
	"user_prompt": "find buyers",
	"items": [
		{
			"id": "obs-1",
			"source_url": "https://www.facebook.com/groups/123/posts/456/",
			"author_profile_url": "https://www.facebook.com/u/1",
			"author_name": "Người Mua",
			"content": "cần tìm xưởng áo thun",
			"reactions": 12, "comments": 3, "shares": 1,
			"post_fbid": "456", "group_fbid": "123",
			"posted_at": "2026-06-01T09:30:00Z"
		}
	],
	"unknown_future_field": "ignored"
}`

// decodeConnectorCrawlResultRequestFromRawJSON unmarshals the raw connector JSON
// into connectorCrawlResultRequest via the same encoding/json path the handler's
// BodyParser uses (numbers → float64, unknown fields ignored).
func decodeConnectorCrawlResultRequestFromRawJSON(t *testing.T, payload string) connectorCrawlResultRequest {
	t.Helper()
	var req connectorCrawlResultRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("decode crawl-result payload: %v", err)
	}
	return req
}

func requireRawCrawlRequestHeader(t *testing.T, req connectorCrawlResultRequest) {
	t.Helper()
	if req.TaskID != "task-77" || req.Intent != "facebook_crawl" || req.AccountID != 4242 {
		t.Errorf("header decode wrong: %+v", req)
	}
	if req.IntentID != 9 || req.Status != "completed" || req.ExitReason != "end_of_feed" {
		t.Errorf("status fields decode wrong: %+v", req)
	}
	if req.UserPrompt != "find buyers" || len(req.Keywords) != 2 || req.Keywords[1] != "áo thun" {
		t.Errorf("keywords/prompt decode wrong: %+v", req)
	}
}

func requireRawCrawlScrollDiag(t *testing.T, diag map[string]any) {
	t.Helper()
	// scroll_diag stays a raw map (numbers as float64 after JSON decode).
	if diag["passes"] != float64(6) || diag["scroll_moved_ever"] != true {
		t.Errorf("scroll_diag decode wrong: %v", diag)
	}
}

func requireRawCrawlMarketSignalGate(t *testing.T, gate map[string]any) {
	t.Helper()
	if gate["min_reactions"] != float64(3) {
		t.Errorf("market_signal_gate decode wrong: %v", gate)
	}
}

func requireRawCrawlItem(t *testing.T, it connectorCrawlItem) {
	t.Helper()
	if it.ID != "obs-1" || it.SourceURL != "https://www.facebook.com/groups/123/posts/456/" {
		t.Errorf("item id/url decode wrong: %+v", it)
	}
	if it.AuthorName != "Người Mua" || it.Content != "cần tìm xưởng áo thun" {
		t.Errorf("item author/content decode wrong: %+v", it)
	}
	if it.Reactions != 12 || it.Comments != 3 || it.Shares != 1 {
		t.Errorf("item engagement decode wrong: %+v", it)
	}
	if it.PostFBID != "456" || it.GroupFBID != "123" || it.PostedAt != "2026-06-01T09:30:00Z" {
		t.Errorf("item fbid/posted_at decode wrong: %+v", it)
	}
}

// TestConnectorCrawlResultRequest_RawJSONWire pins the connector crawl-result WIRE
// contract: a raw JSON payload exactly as the Chrome extension posts it decodes
// into connectorCrawlResultRequest / connectorCrawlItem by their json tags, with
// the documented types (account_id int64, scroll_diag map, items array, unknown
// fields ignored). This guards the DTO json tags/field names independently of the
// ingest pipeline (which the path tests drive separately).
func TestConnectorCrawlResultRequest_RawJSONWire(t *testing.T) {
	req := decodeConnectorCrawlResultRequestFromRawJSON(t, rawCrawlResultPayload)

	requireRawCrawlRequestHeader(t, req)
	requireRawCrawlScrollDiag(t, req.ScrollDiag)
	requireRawCrawlMarketSignalGate(t, req.MarketSignalGate)
	if len(req.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(req.Items))
	}
	requireRawCrawlItem(t, req.Items[0])
}
