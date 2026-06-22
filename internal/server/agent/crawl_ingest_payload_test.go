package agent

import (
	"encoding/json"
	"testing"
)

// TestConnectorCrawlResultRequest_RawJSONWire pins the connector crawl-result WIRE
// contract: a raw JSON payload exactly as the Chrome extension posts it decodes
// into connectorCrawlResultRequest / connectorCrawlItem by their json tags, with
// the documented types (account_id int64, scroll_diag map, items array, unknown
// fields ignored). This guards the DTO json tags/field names independently of the
// ingest pipeline (which the path tests drive separately).
func TestConnectorCrawlResultRequest_RawJSONWire(t *testing.T) {
	const payload = `{
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

	var req connectorCrawlResultRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("decode crawl-result payload: %v", err)
	}

	if req.TaskID != "task-77" || req.Intent != "facebook_crawl" || req.AccountID != 4242 {
		t.Errorf("header decode wrong: %+v", req)
	}
	if req.IntentID != 9 || req.Status != "completed" || req.ExitReason != "end_of_feed" {
		t.Errorf("status fields decode wrong: %+v", req)
	}
	if req.UserPrompt != "find buyers" || len(req.Keywords) != 2 || req.Keywords[1] != "áo thun" {
		t.Errorf("keywords/prompt decode wrong: %+v", req)
	}
	// scroll_diag stays a raw map (numbers as float64 after JSON decode).
	if req.ScrollDiag["passes"] != float64(6) || req.ScrollDiag["scroll_moved_ever"] != true {
		t.Errorf("scroll_diag decode wrong: %v", req.ScrollDiag)
	}
	if req.MarketSignalGate["min_reactions"] != float64(3) {
		t.Errorf("market_signal_gate decode wrong: %v", req.MarketSignalGate)
	}
	if len(req.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(req.Items))
	}
	it := req.Items[0]
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
