package store

import (
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/fburl"
	"github.com/thg/scraper/internal/models"
)

// repairLeadSourceURL is the read-time twin of the ingest-time URL rescue.
// These tests pin its behaviour against the cases that produced the
// "Mở bài viết → newsfeed" bug in production:
//   - source_url is a group/profile shell, post_fbid + group_fbid present.
//   - source_url is malformed but contains an embedded post id.
//   - source_url is already canonical (must not be rewritten).
func TestRepairLeadSourceURL(t *testing.T) {
	cases := []struct {
		name string
		in   models.Lead
		want string
	}{
		{
			name: "group shell + IDs → canonical permalink",
			in: models.Lead{
				SourceURL: "https://www.facebook.com/groups/123",
				PostFBID:  "456",
				GroupFBID: "123",
			},
			want: "https://www.facebook.com/groups/123/posts/456/",
		},
		{
			name: "profile URL + post_fbid only → permalink.php fallback",
			in: models.Lead{
				SourceURL: "https://www.facebook.com/michael.kim",
				PostFBID:  "789",
			},
			want: "https://www.facebook.com/permalink.php?story_fbid=789",
		},
		{
			name: "URL with embedded story_fbid recovers without explicit ID columns",
			in: models.Lead{
				SourceURL: "https://www.facebook.com/?story_fbid=111&id=2",
			},
			// Already looks like a post URL — repair is a no-op.
			want: "https://www.facebook.com/?story_fbid=111&id=2",
		},
		{
			name: "already canonical permalink → untouched",
			in: models.Lead{
				SourceURL: "https://www.facebook.com/groups/1/posts/2",
				PostFBID:  "2",
				GroupFBID: "1",
			},
			want: "https://www.facebook.com/groups/1/posts/2",
		},
		{
			name: "group shell with no PostFBID stays unrepaired (no rescue possible)",
			in: models.Lead{
				SourceURL: "https://www.facebook.com/groups/123",
			},
			want: "https://www.facebook.com/groups/123",
		},
		{
			name: "URL contains a comment_id with parent post — already a post URL, no rescue",
			in: models.Lead{
				SourceURL: "https://www.facebook.com/groups/1/posts/2?comment_id=9",
				PostFBID:  "2",
				GroupFBID: "1",
			},
			want: "https://www.facebook.com/groups/1/posts/2?comment_id=9",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := tc.in
			repairLeadSourceURL(&l)
			if l.SourceURL != tc.want {
				t.Errorf("SourceURL = %q, want %q", l.SourceURL, tc.want)
			}
		})
	}
}

// TestRepairLeadSourceURL_NilGuard ensures a nil pointer does not panic.
// The store-layer scan loop holds Lead by value, but the guard is cheap
// and protects against future callers.
func TestRepairLeadSourceURL_NilGuard(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil lead panicked: %v", r)
		}
	}()
	repairLeadSourceURL(nil)
}

// TestGetLeadsFiltered_RescuesStaleSourceURL is the end-to-end variant:
// seed a row whose source_url is a group shell but post_fbid + group_fbid
// are populated, then read it back through the API and confirm the
// dashboard would receive a canonical permalink.
func TestGetLeadsFiltered_RescuesStaleSourceURL(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "leads_repair.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	// Direct INSERT to simulate a legacy row written before the contract
	// fix landed. Bypasses the ingest path on purpose — that path now
	// rescues at write time too, so a fresh ingest could not produce this.
	if _, err := db.db.Exec(
		`INSERT INTO leads (org_id, source_type, source_id, source_url, post_fbid, group_fbid,
		                    platform, author, author_url, content, score, service_match,
		                    author_role, pain_point, ai_reasoning, niche, classified_at)
		 VALUES (1, 'post', 0, ?, '456', '123', 'facebook', 'Michael Kim', '', 'looking for fulfill',
		         'warm', 'None', 'unknown', '', '', 'logistics', CURRENT_TIMESTAMP)`,
		"https://www.facebook.com/groups/123", // the shell URL
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	leads, err := db.GetLeadsFiltered("", "", 50, 0, 1)
	if err != nil {
		t.Fatalf("GetLeadsFiltered: %v", err)
	}
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	got := leads[0].SourceURL
	want := "https://www.facebook.com/groups/123/posts/456/"
	if got != want {
		t.Errorf("read source_url = %q, want %q (repair should have synthesised the post permalink)", got, want)
	}
	if !fburl.LooksLikePostURL(got) {
		t.Errorf("repaired URL must pass LooksLikePostURL; got %q", got)
	}
}
