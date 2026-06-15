package leads_test

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

const (
	dprOrg       = int64(7)
	dprPostFBID  = "4505595319766639"
	dprCanonical = "https://www.facebook.com/groups/ship.viet.my/permalink/4505595319766639/"
	dprGroupRef  = "ship.viet.my"
)

func mkPostLead(t *testing.T, db *store.Store, sourceURL, groupFBID, content string) {
	t.Helper()
	if _, err := db.Leads().InsertLead(&models.Lead{
		OrgID: dprOrg, SourceType: "post", SourceURL: sourceURL, PostFBID: dprPostFBID, GroupFBID: groupFBID,
		Platform: models.PlatformFacebook, Author: "x", Content: content, Score: models.LeadHot,
	}); err != nil {
		t.Fatalf("InsertLead(%s): %v", sourceURL, err)
	}
}

// The production identity-mismatch bug + the fix, at the data layer: a direct-post
// lookup must NEVER attach a generic permalink.php lead, a different-group lead, or an
// UNPROVABLE numeric-group lead that merely shares the post id; it matches ONLY the
// exact canonical URL or a lead in the SAME group ref.
func TestGetPostLeadByDirectPostRef_GroupContext(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapLeadsStore, "direct_post_ref")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// 1. A generic permalink.php DECOY (the wrong Data-Engineer post) → NO safe match,
	//    and it IS flagged as a definite conflict.
	mkPostLead(t, db, "https://www.facebook.com/permalink.php?story_fbid=4505595319766639", "", "Data Engineer job post")
	if l, _ := db.Leads().GetPostLeadByDirectPostRef(ctx, dprOrg, dprPostFBID, dprCanonical, dprGroupRef); l != nil {
		t.Errorf("generic permalink.php lead must NOT match a group direct-post, got id=%d url=%q", l.ID, l.SourceURL)
	}
	if d, _ := db.Leads().FindConflictingPostLead(ctx, dprOrg, dprPostFBID, dprCanonical, dprGroupRef); d == nil {
		t.Error("the permalink.php decoy must be reported as a definite conflict")
	}

	// 2. A DIFFERENT named group with the same post id → still no match (definite conflict).
	mkPostLead(t, db, "https://www.facebook.com/groups/other.group/permalink/4505595319766639/", "other.group", "unrelated")
	if l, _ := db.Leads().GetPostLeadByDirectPostRef(ctx, dprOrg, dprPostFBID, dprCanonical, dprGroupRef); l != nil {
		t.Errorf("different-group lead must NOT match, got id=%d url=%q", l.ID, l.SourceURL)
	}

	// 3. A numeric-group lead sharing the post id → STILL no match: a vanity→numeric
	//    redirect and an unrelated numeric group are indistinguishable without import
	//    provenance, so we refuse to attach it (safety over a possibly-right guess).
	mkPostLead(t, db, "https://www.facebook.com/groups/123456789/permalink/4505595319766639/", "123456789", "frozen goods to Texas")
	if l, _ := db.Leads().GetPostLeadByDirectPostRef(ctx, dprOrg, dprPostFBID, dprCanonical, dprGroupRef); l != nil {
		t.Errorf("unprovable numeric-group lead must NOT match without exact canonical, got id=%d url=%q", l.ID, l.SourceURL)
	}

	// 4. The exact canonical (vanity) lead — the provable same post — wins outright.
	mkPostLead(t, db, dprCanonical, dprGroupRef, "frozen goods to Texas")
	got, _ := db.Leads().GetPostLeadByDirectPostRef(ctx, dprOrg, dprPostFBID, dprCanonical, dprGroupRef)
	if got == nil || got.SourceURL != dprCanonical {
		t.Errorf("exact canonical must match, got %+v", got)
	}

	// Org isolation.
	if other, _ := db.Leads().GetPostLeadByDirectPostRef(ctx, 8, dprPostFBID, dprCanonical, dprGroupRef); other != nil {
		t.Errorf("org 8 must not see org 7's lead")
	}
}

// A numeric-group lead sharing the post id is AMBIGUOUS — possibly a vanity→numeric
// redirect of the same post, possibly an unrelated collision. It must be neither
// matched NOR asserted as a hard identity_mismatch: the poller degrades to retry
// (lead_not_observed), never a wrong comment and never a false mismatch claim.
func TestDirectPostRef_NumericGroupIsAmbiguous(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapLeadsStore, "direct_post_ref_numeric")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mkPostLead(t, db, "https://www.facebook.com/groups/123456789/permalink/4505595319766639/", "123456789", "frozen goods to Texas")

	if l, _ := db.Leads().GetPostLeadByDirectPostRef(ctx, dprOrg, dprPostFBID, dprCanonical, dprGroupRef); l != nil {
		t.Errorf("numeric-group lead must NOT match a vanity request without provenance, got id=%d", l.ID)
	}
	if d, _ := db.Leads().FindConflictingPostLead(ctx, dprOrg, dprPostFBID, dprCanonical, dprGroupRef); d != nil {
		t.Errorf("numeric-group lead must NOT be a hard conflict (ambiguous → retry), got id=%d url=%q", d.ID, d.SourceURL)
	}
}
