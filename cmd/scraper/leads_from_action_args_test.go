package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/store"
)

// Integration tests for the fbLeadSource adapter wiring facebook.LeadsForAction to
// the REAL store (PR29G). The pure routing/precedence/synthetic-shaping behavior is
// covered store-free in internal/services/facebook/outbound_lead_selection_test.go;
// these keep a real-store guard on the adapter + tenant isolation (binding), which a
// fake cannot prove — GetLeadByID's org_id filter must reject a wrong-org lead_id.

func newArgsStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "leads_args_test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// lead_id path through the real adapter resolves the ONE existing org-scoped lead.
func TestLeadsForActionAdapter_LeadIDPath(t *testing.T) {
	ctx := context.Background()
	db := newArgsStore(t)
	const orgID = int64(5)
	id, err := db.Leads().InsertLead(&models.Lead{
		OrgID: orgID, SourceType: "post", SourceURL: "https://www.facebook.com/groups/1/posts/2/",
		Platform: models.PlatformFacebook, Author: "An", Content: "real lead content", Score: models.LeadHot,
	})
	if err != nil {
		t.Fatalf("InsertLead: %v", err)
	}

	leads, err := facebook.LeadsForAction(ctx, fbLeadSource{db}, orgID, "comment", facebook.LeadSelectionInput{LeadID: id})
	if err != nil {
		t.Fatalf("LeadsForAction: %v", err)
	}
	if len(leads) != 1 || leads[0].ID != id || leads[0].Content != "real lead content" {
		t.Fatalf("lead_id path must return the one real lead, got %+v", leads)
	}
}

// Tenant isolation (binding): a lead_id from another org returns no leads — the
// real GetLeadByID filters by org_id, so a wrong-org id is "not found", never a
// cross-tenant read.
func TestLeadsForActionAdapter_LeadIDTenantScoped(t *testing.T) {
	ctx := context.Background()
	db := newArgsStore(t)
	id, err := db.Leads().InsertLead(&models.Lead{
		OrgID: 5, SourceType: "post", SourceURL: "https://www.facebook.com/groups/1/posts/2/",
		Platform: models.PlatformFacebook, Content: "org5 lead", Score: models.LeadHot,
	})
	if err != nil {
		t.Fatalf("InsertLead: %v", err)
	}
	// Same lead_id, but a DIFFERENT org → not found → no leads (no cross-tenant leak).
	leads, err := facebook.LeadsForAction(ctx, fbLeadSource{db}, 6, "comment", facebook.LeadSelectionInput{LeadID: id})
	if err != nil {
		t.Fatalf("LeadsForAction: %v", err)
	}
	if len(leads) != 0 {
		t.Fatalf("lead_id from another org must yield no leads, got %+v", leads)
	}
}

// No lead_id and no prompt target → WorkQueue via the real adapter (empty store →
// empty result, the "no eligible lead" path).
func TestLeadsForActionAdapter_WorkQueueFallthrough(t *testing.T) {
	ctx := context.Background()
	db := newArgsStore(t)
	leads, err := facebook.LeadsForAction(ctx, fbLeadSource{db}, 5, "comment", facebook.LeadSelectionInput{})
	if err != nil {
		t.Fatalf("LeadsForAction: %v", err)
	}
	if len(leads) != 0 {
		t.Fatalf("no target + empty work queue must yield no leads, got %+v", leads)
	}
}
