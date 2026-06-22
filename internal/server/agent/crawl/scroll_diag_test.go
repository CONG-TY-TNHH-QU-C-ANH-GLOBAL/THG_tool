package crawl

import (
	"encoding/json"
	"testing"
)

// The extension's scroll_diag is JSON-decoded into map[string]any, so numeric fields can arrive
// as float64 / json.Number / int / string, and any field can be missing or null. These must
// never panic — blind type assertions (the thing we are forbidding) would.
func TestScrollDiagInt_AllForms(t *testing.T) {
	cases := []struct {
		name string
		diag map[string]any
		want int
	}{
		{"nil map", nil, 0},
		{"missing key", map[string]any{"other": 1}, 0},
		{"null value", map[string]any{"max_articles_seen": nil}, 0},
		{"int", map[string]any{"max_articles_seen": 7}, 7},
		{"int64", map[string]any{"max_articles_seen": int64(9)}, 9},
		{"float64 (default JSON number)", map[string]any{"max_articles_seen": float64(12)}, 12},
		{"json.Number", map[string]any{"max_articles_seen": json.Number("15")}, 15},
		{"json.Number float", map[string]any{"max_articles_seen": json.Number("15.9")}, 15},
		{"numeric string", map[string]any{"max_articles_seen": "21"}, 21},
		{"garbage string", map[string]any{"max_articles_seen": "not-a-number"}, 0},
		{"bool (wrong type)", map[string]any{"max_articles_seen": true}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scrollDiagInt(tc.diag, "max_articles_seen"); got != tc.want {
				t.Errorf("scrollDiagInt = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestScrollDiagBool_AllForms(t *testing.T) {
	cases := []struct {
		name string
		diag map[string]any
		want bool
	}{
		{"nil map", nil, false},
		{"missing key", map[string]any{}, false},
		{"null", map[string]any{"scroll_moved_ever": nil}, false},
		{"true bool", map[string]any{"scroll_moved_ever": true}, true},
		{"false bool", map[string]any{"scroll_moved_ever": false}, false},
		{"true string", map[string]any{"scroll_moved_ever": "true"}, true},
		{"1 string", map[string]any{"scroll_moved_ever": "1"}, true},
		{"numeric wrong type", map[string]any{"scroll_moved_ever": float64(1)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scrollDiagBool(tc.diag, "scroll_moved_ever"); got != tc.want {
				t.Errorf("scrollDiagBool = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestScrollDiagString_AllForms(t *testing.T) {
	if got := scrollDiagString(nil, "final_scroll_target"); got != "" {
		t.Errorf("nil map → %q, want empty", got)
	}
	if got := scrollDiagString(map[string]any{"final_scroll_target": 5}, "final_scroll_target"); got != "" {
		t.Errorf("wrong type → %q, want empty", got)
	}
	if got := scrollDiagString(map[string]any{"final_scroll_target": "  window  "}, "final_scroll_target"); got != "window" {
		t.Errorf("string → %q, want %q", got, "window")
	}
}

// NormalizeConnectorScrollDiag must produce the zero value (never panic) for the phantom-DOM
// payload shape (max_articles_seen=0, max_doc_height=0, scroll_moved_ever=false) AND for a
// completely nil map, and must coerce mixed JSON types.
func TestNormalizeConnectorScrollDiag(t *testing.T) {
	if got := NormalizeConnectorScrollDiag(nil); got != (ConnectorScrollDiag{}) {
		t.Errorf("nil map must yield zero value, got %+v", got)
	}
	// Shape mirroring a real JSON decode (numbers as float64) — the phantom case.
	phantom := map[string]any{
		"max_articles_seen": float64(0), "max_doc_height": float64(0),
		"scroll_moved_ever": false, "passes": float64(8), "final_scroll_target": "document",
	}
	got := NormalizeConnectorScrollDiag(phantom)
	if got.MaxArticlesSeen != 0 || got.MaxDocHeight != 0 || got.ScrollMovedEver {
		t.Errorf("phantom diag mis-normalized: %+v", got)
	}
	if got.Passes != 8 || got.FinalScrollTarget != "document" {
		t.Errorf("normalized passes/target wrong: %+v", got)
	}
}
