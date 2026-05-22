// Domain: leads × store integration (cross-domain).
package store

import (
	"testing"

	"github.com/thg/scraper/internal/fburl"
)

// TestGetLeadsFiltered_RescuesStaleSourceURL is the end-to-end variant:
// seed a row whose source_url is a group shell but post_fbid + group_fbid
// are populated, then read it back through the API and confirm the
// dashboard would receive a canonical permalink.
//
// Stays at top-level: writes via raw db.db.Exec to plant a legacy row,
// reads back via the leads subpackage's public GetLeadsFiltered. The
// unexported-helper round-trip lives in
// internal/store/leads/leads_repair_internal_test.go.
func TestGetLeadsFiltered_RescuesStaleSourceURL(t *testing.T) {
	db := newSharedStore(t, "leads_repair.db")

	// Direct INSERT to simulate a legacy row written before the contract
	// fix landed. Bypasses the ingest path on purpose — that path now
	// rescues at write time too, so a fresh ingest could not produce this.
	if _, err := db.db.Exec(
		`INSERT INTO leads (org_id, source_type, source_id, source_url, post_fbid, group_fbid,
		                    platform, author, author_url, content, score, service_match,
		                    author_role, pain_point, ai_reasoning, niche, classified_at)
		 VALUES (1, 'post', 0, ?, '456', '123', 'facebook', 'Michael Kim', '', 'looking for fulfill',
		         'warm', 'None', 'unknown', '', '', 'logistics', CURRENT_TIMESTAMP)`,
		"https://www.facebook.com/groups/123",
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	leads, err := db.Leads().GetLeadsFiltered("", "", 50, 0, 1)
	if err != nil {
		t.Fatalf("GetLeadsFiltered: %v", err)
	}
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	got := leads[0].SourceURL
	want := "https://www.facebook.com/groups/123/permalink/456/"
	if got != want {
		t.Errorf("read source_url = %q, want %q (repair should have synthesised the post permalink)", got, want)
	}
	if !fburl.LooksLikePostURL(got) {
		t.Errorf("repaired URL must pass LooksLikePostURL; got %q", got)
	}
}
