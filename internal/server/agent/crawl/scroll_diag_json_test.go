package crawl

import (
	"encoding/json"
	"testing"
)

// TestNormalizeConnectorScrollDiag_RawJSONWire pins the WIRE contract: a raw
// scroll_diag JSON payload (exactly as the Chrome extension posts it inside a
// crawl-result) is decoded with encoding/json — so all numbers arrive as
// float64 — and normalized into the typed view by input key name. This proves
// the extraction preserved the key names and type coercion the connector relies
// on (not just Go-map → Go-struct behavior).
func TestNormalizeConnectorScrollDiag_RawJSONWire(t *testing.T) {
	const payload = `{
		"passes": 6,
		"max_articles_seen": 14,
		"max_scroll_y": 8200,
		"max_doc_height": 19000,
		"scroll_moved_ever": true,
		"final_scroll_target": "  window  ",
		"landed_url": "https://www.facebook.com/groups/123/posts/456/",
		"unknown_future_field": "ignored"
	}`
	var diag map[string]any
	if err := json.Unmarshal([]byte(payload), &diag); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	got := NormalizeConnectorScrollDiag(diag)
	want := ConnectorScrollDiag{
		Passes:            6,
		MaxArticlesSeen:   14,
		MaxScrollY:        8200,
		MaxDocHeight:      19000,
		ScrollMovedEver:   true,
		FinalScrollTarget: "window", // trimmed
		LandedURL:         "https://www.facebook.com/groups/123/posts/456/",
	}
	if got != want {
		t.Errorf("raw-JSON scroll_diag normalized to %+v, want %+v", got, want)
	}

	// A connector that omits scroll_diag entirely (null) must yield the zero
	// value, never a panic — the phantom-DOM safety contract.
	var nullDiag map[string]any
	_ = json.Unmarshal([]byte(`null`), &nullDiag)
	if got := NormalizeConnectorScrollDiag(nullDiag); got != (ConnectorScrollDiag{}) {
		t.Errorf("null scroll_diag must yield zero value, got %+v", got)
	}
}
