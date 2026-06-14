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
