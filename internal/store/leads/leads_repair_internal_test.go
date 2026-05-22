// Internal-package test: needs reach to unexported repairLeadSourceURL.
// Per [[feedback_subpackage_contract]] rule 9 the bulk of leads tests
// live in `package leads_test` / top-level; this is the narrow exception
// for the unexported-helper round-trip.
package leads

import (
	"testing"

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
			want: "https://www.facebook.com/groups/123/permalink/456/",
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
func TestRepairLeadSourceURL_NilGuard(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil lead panicked: %v", r)
		}
	}()
	repairLeadSourceURL(nil)
}
