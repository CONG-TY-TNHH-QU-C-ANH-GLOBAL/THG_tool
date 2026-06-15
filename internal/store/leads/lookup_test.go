package leads_test

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapLeadsStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func TestGetLeadByPostRefAndID(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapLeadsStore, "leads_lookup")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgID = int64(7)

	id, err := db.Leads().InsertLead(&models.Lead{
		OrgID: orgID, SourceType: "post",
		SourceURL: "https://www.facebook.com/groups/123/permalink/456/",
		PostFBID:  "456", GroupFBID: "123",
		Platform: models.PlatformFacebook, Author: "An", Content: "ai làm fulfill US",
		Score: models.LeadHot,
	})
	if err != nil || id <= 0 {
		t.Fatalf("InsertLead: id=%d err=%v", id, err)
	}

	// Found by post_fbid (the canonical id is stable across URL shapes), with content.
	got, err := db.Leads().GetLeadByPostRef(ctx, orgID, "456", "https://www.facebook.com/groups/123/permalink/456/")
	if err != nil {
		t.Fatalf("GetLeadByPostRef: %v", err)
	}
	if got == nil || got.ID != id || got.Content != "ai làm fulfill US" {
		t.Fatalf("expected lead %d with content, got %+v", id, got)
	}

	// Unknown post → (nil, nil): the caller must NOT fabricate post content.
	miss, err := db.Leads().GetLeadByPostRef(ctx, orgID, "999", "https://www.facebook.com/groups/123/permalink/999/")
	if err != nil {
		t.Fatalf("miss err: %v", err)
	}
	if miss != nil {
		t.Fatalf("expected nil for unknown post, got %+v", miss)
	}

	// Tenant isolation: another org cannot see this lead.
	if other, _ := db.Leads().GetLeadByPostRef(ctx, 999, "456", ""); other != nil {
		t.Fatalf("org isolation breached: %+v", other)
	}

	// GetLeadByID round-trips the same lead.
	byID, err := db.Leads().GetLeadByID(ctx, orgID, id)
	if err != nil || byID == nil || byID.ID != id {
		t.Fatalf("GetLeadByID: err=%v lead=%+v", err, byID)
	}
}

// Legacy URL shapes: a lead crawled with a mobile/tracking/trailing-slash
// source_url is still found by the CANONICAL post id (post_fbid is stable across
// URL shapes). When post_fbid is missing AND the stored URL differs from the
// canonical form, the lookup misses — a documented limitation, never fabricated.
func TestGetLeadByPostRef_LegacyURLShapes(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapLeadsStore, "leads_lookup_legacy")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgID = int64(7)

	// Messy stored URL (mobile host + tracking params) but a clean post_fbid.
	id, err := db.Leads().InsertLead(&models.Lead{
		OrgID: orgID, SourceType: "post",
		SourceURL: "https://m.facebook.com/groups/123/posts/456/?ref=share&mibextid=x",
		PostFBID:  "456", GroupFBID: "123",
		Platform: models.PlatformFacebook, Author: "An", Content: "x", Score: models.LeadHot,
	})
	if err != nil || id <= 0 {
		t.Fatalf("InsertLead: id=%d err=%v", id, err)
	}
	// Canonical lookup finds it via the stable post id despite the messy stored URL.
	got, err := db.Leads().GetLeadByPostRef(ctx, orgID, "456", "https://www.facebook.com/groups/123/permalink/456/")
	if err != nil || got == nil || got.ID != id {
		t.Fatalf("legacy-shaped lead must be found by post_fbid: err=%v lead=%+v", err, got)
	}

	// Documented limitation: no post_fbid + a stored URL unequal to the canonical
	// form → miss (nil, nil). The caller asks the user to scan/import; it never
	// fabricates post content.
	if _, err := db.Leads().InsertLead(&models.Lead{
		OrgID: orgID, SourceType: "post",
		SourceURL: "https://m.facebook.com/story.php?story_fbid=789&id=1", PostFBID: "",
		Platform: models.PlatformFacebook, Author: "B", Content: "y", Score: models.LeadHot,
	}); err != nil {
		t.Fatalf("InsertLead 2: %v", err)
	}
	miss, err := db.Leads().GetLeadByPostRef(ctx, orgID, "789", "https://www.facebook.com/permalink.php?story_fbid=789")
	if err != nil {
		t.Fatalf("limitation lookup err: %v", err)
	}
	if miss != nil {
		t.Fatalf("documented limitation expected miss (no post_fbid + non-canonical URL), got %+v", miss)
	}
}

// GetPostLeadByRef returns the POST lead only — never a commenter lead that shares
// the same post ref — excludes archived, and is org-isolated.
func TestGetPostLeadByRef(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapLeadsStore, "leads_post_ref")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgID = int64(7)
	const canonical = "https://www.facebook.com/groups/123/permalink/456/"

	// A commenter lead sharing the same post_fbid/source_url (created first).
	if _, err := db.Leads().InsertLead(&models.Lead{
		OrgID: orgID, SourceType: "comment", SourceURL: canonical, SecondaryURL: canonical + "?comment_id=9",
		PostFBID: "456", CommentFBID: "9", GroupFBID: "123", Platform: models.PlatformFacebook,
		Author: "Commenter", Content: "tôi cũng làm dịch vụ này", Score: models.LeadWarm,
	}); err != nil {
		t.Fatalf("InsertLead commenter: %v", err)
	}
	// The post author's lead.
	postID, err := db.Leads().InsertLead(&models.Lead{
		OrgID: orgID, SourceType: "post", SourceURL: canonical, PostFBID: "456", GroupFBID: "123",
		Platform: models.PlatformFacebook, Author: "An", Content: "ai làm fulfill US", Score: models.LeadHot,
	})
	if err != nil || postID <= 0 {
		t.Fatalf("InsertLead post: id=%d err=%v", postID, err)
	}

	// Returns the POST lead, not the commenter.
	got, err := db.Leads().GetPostLeadByRef(ctx, orgID, "456", canonical)
	if err != nil || got == nil {
		t.Fatalf("GetPostLeadByRef: got=%v err=%v", got, err)
	}
	if got.ID != postID || got.SourceType != "post" || got.Author != "An" {
		t.Errorf("expected the post lead (id=%d author=An), got id=%d source=%s author=%s", postID, got.ID, got.SourceType, got.Author)
	}

	// Org isolation: a different org sees nothing.
	if other, _ := db.Leads().GetPostLeadByRef(ctx, 8, "456", canonical); other != nil {
		t.Errorf("org 8 must not see org 7's post lead, got %+v", other)
	}

	// Archived post lead is excluded.
	if err := db.Leads().ArchiveLead(ctx, orgID, postID, "test"); err != nil {
		t.Fatalf("ArchiveLead: %v", err)
	}
	if got, _ := db.Leads().GetPostLeadByRef(ctx, orgID, "456", canonical); got != nil {
		t.Errorf("archived post lead must be excluded, got id=%d source=%s", got.ID, got.SourceType)
	}
}
