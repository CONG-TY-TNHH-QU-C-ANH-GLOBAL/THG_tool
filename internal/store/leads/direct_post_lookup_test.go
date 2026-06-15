package leads_test

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

// The production identity-mismatch bug + the fix, at the data layer: a direct-post
// lookup must NEVER attach a generic permalink.php lead or a different-group lead that
// merely shares the post id, but MUST accept the exact post and a vanity→numeric group
// redirect of the same post.
func TestGetPostLeadByDirectPostRef_GroupContext(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapLeadsStore, "direct_post_ref")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const org = int64(7)
	const postFBID = "4505595319766639"
	const canonical = "https://www.facebook.com/groups/ship.viet.my/permalink/4505595319766639/"
	const groupRef = "ship.viet.my"

	mk := func(sourceURL, groupFBID, content string) {
		if _, err := db.Leads().InsertLead(&models.Lead{
			OrgID: org, SourceType: "post", SourceURL: sourceURL, PostFBID: postFBID, GroupFBID: groupFBID,
			Platform: models.PlatformFacebook, Author: "x", Content: content, Score: models.LeadHot,
		}); err != nil {
			t.Fatalf("InsertLead(%s): %v", sourceURL, err)
		}
	}

	// 1. Only a generic permalink.php DECOY (the wrong Data-Engineer post) exists →
	//    NO safe match, and it is flagged as the conflict.
	mk("https://www.facebook.com/permalink.php?story_fbid=4505595319766639", "", "Data Engineer job post")
	if l, _ := db.Leads().GetPostLeadByDirectPostRef(ctx, org, postFBID, canonical, groupRef); l != nil {
		t.Errorf("generic permalink.php lead must NOT match a group direct-post, got id=%d url=%q", l.ID, l.SourceURL)
	}
	if d, _ := db.Leads().FindConflictingPostLead(ctx, org, postFBID, canonical, groupRef); d == nil {
		t.Error("the permalink.php decoy must be reported as a conflict")
	}

	// 2. A DIFFERENT named group with the same post id → still no match.
	mk("https://www.facebook.com/groups/other.group/permalink/4505595319766639/", "other.group", "unrelated")
	if l, _ := db.Leads().GetPostLeadByDirectPostRef(ctx, org, postFBID, canonical, groupRef); l != nil {
		t.Errorf("different-group lead must NOT match, got id=%d url=%q", l.ID, l.SourceURL)
	}

	// 3. A vanity→numeric redirect of the SAME post (group stored numerically) → MATCH.
	mk("https://www.facebook.com/groups/123456789/permalink/4505595319766639/", "123456789", "frozen goods to Texas")
	got, _ := db.Leads().GetPostLeadByDirectPostRef(ctx, org, postFBID, canonical, groupRef)
	if got == nil || !isGroupPermalink(got.SourceURL) {
		t.Errorf("vanity→numeric redirect of the same post must match, got %+v", got)
	}

	// 4. The exact canonical (vanity) lead wins outright.
	mk(canonical, groupRef, "frozen goods to Texas")
	got2, _ := db.Leads().GetPostLeadByDirectPostRef(ctx, org, postFBID, canonical, groupRef)
	if got2 == nil || got2.SourceURL != canonical {
		t.Errorf("exact canonical must match, got %+v", got2)
	}

	// Org isolation.
	if other, _ := db.Leads().GetPostLeadByDirectPostRef(ctx, 8, postFBID, canonical, groupRef); other != nil {
		t.Errorf("org 8 must not see org 7's lead")
	}
}

func isGroupPermalink(u string) bool {
	return len(u) > 0 && (containsAll(u, "/groups/", "/permalink/") || containsAll(u, "/groups/", "/posts/"))
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
