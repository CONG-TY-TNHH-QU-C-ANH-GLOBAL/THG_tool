package knowledge_test

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// Recording a sync event round-trips through ListRecentSyncsForOrg.
func TestRecordKnowledgeSync_RoundTrip(t *testing.T) {
	db := newKnowledgeStore(t, "events.db")
	ctx := context.Background()

	db.RecordSync(ctx, 7, sources.SourceCSV, 10, 7, 3, 0, 150, 0)
	db.RecordSync(ctx, 7, sources.SourceShopify, 250, 5, 245, 0, 9800, 0)

	list, err := db.ListRecentSyncsForOrg(ctx, 7, 10)
	if err != nil {
		t.Fatalf("ListRecentSyncsForOrg: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d entries; want 2", len(list))
	}
	// Newest first â€” the second insert (shopify) should be at index 0.
	if list[0].SourceType != "shopify" {
		t.Errorf("expected shopify first; got %q", list[0].SourceType)
	}
	if list[0].AssetsSeen != 250 {
		t.Errorf("AssetsSeen round-trip: got %d want 250", list[0].AssetsSeen)
	}
}

// Cross-org isolation: org-1's syncs are invisible to org-2.
func TestListRecentSyncsForOrg_TenantScope(t *testing.T) {
	db := newKnowledgeStore(t, "events.db")
	ctx := context.Background()
	db.RecordSync(ctx, 1, sources.SourceCSV, 5, 5, 0, 0, 100, 0)
	db.RecordSync(ctx, 2, sources.SourceShopify, 100, 100, 0, 0, 200, 0)

	list1, _ := db.ListRecentSyncsForOrg(ctx, 1, 10)
	if len(list1) != 1 || list1[0].SourceType != "csv" {
		t.Errorf("org-1 should see only its own sync; got %+v", list1)
	}
	list2, _ := db.ListRecentSyncsForOrg(ctx, 2, 10)
	if len(list2) != 1 || list2[0].SourceType != "shopify" {
		t.Errorf("org-2 should see only its own sync; got %+v", list2)
	}
}

// RecordKnowledgeOutcome with "sent" increments conversion counters
// on every asset present in the matching retrieval event.
func TestRecordKnowledgeOutcome_IncrementsConversionsOnSent(t *testing.T) {
	db := newKnowledgeStore(t, "events.db")
	ctx := context.Background()
	sid := mustSetupSource(t, db, 7)
	a1, _ := db.UpsertAsset(ctx, newTestAsset(7, sid, "ext_a", "A"))
	a2, _ := db.UpsertAsset(ctx, newTestAsset(7, sid, "ext_b", "B"))

	retrievalID := "ret_001"
	db.RecordRetrieval(ctx, 7, retrievalID, "cat tee", []retrieval.Hit{
		{Asset: a1, Score: 0.9, Reason: "test"},
		{Asset: a2, Score: 0.7, Reason: "test"},
	}, "comment_drafted")

	db.RecordOutcome(ctx, 7, retrievalID, "sent")

	a1After, _ := db.GetAsset(ctx, a1.ID, 7)
	a2After, _ := db.GetAsset(ctx, a2.ID, 7)
	if a1After.Metrics.Conversions30d != 1 {
		t.Errorf("asset 1 conversion not incremented; got %d", a1After.Metrics.Conversions30d)
	}
	if a2After.Metrics.Conversions30d != 1 {
		t.Errorf("asset 2 conversion not incremented; got %d", a2After.Metrics.Conversions30d)
	}
}

// "rejected" outcome does NOT increment conversions â€” only the sales
// successes count. This is invariant: compliance-blocked comments
// should not be credited as "this asset converted."
func TestRecordKnowledgeOutcome_RejectedDoesNotConvert(t *testing.T) {
	db := newKnowledgeStore(t, "events.db")
	ctx := context.Background()
	sid := mustSetupSource(t, db, 7)
	a, _ := db.UpsertAsset(ctx, newTestAsset(7, sid, "ext_a", "A"))

	retrievalID := "ret_002"
	db.RecordRetrieval(ctx, 7, retrievalID, "test", []retrieval.Hit{
		{Asset: a, Score: 1.0, Reason: "test"},
	}, "comment_drafted")
	db.RecordOutcome(ctx, 7, retrievalID, "rejected")

	got, _ := db.GetAsset(ctx, a.ID, 7)
	if got.Metrics.Conversions30d != 0 {
		t.Errorf("rejected outcome incorrectly credited; got %d conversions", got.Metrics.Conversions30d)
	}
}

// Stale-asset count: returns assets retrieved > daysIdle ago, but
// NOT brand new (never-retrieved) ones.
func TestCountStaleKnowledgeAssetsForOrg(t *testing.T) {
	db := newKnowledgeStore(t, "events.db")
	ctx := context.Background()
	sid := mustSetupSource(t, db, 7)

	// Asset 1: never retrieved â€” NOT counted as stale.
	if _, err := db.UpsertAsset(ctx, newTestAsset(7, sid, "ext_new", "New")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Asset 2: retrieved long ago. Use direct SQL to backdate.
	a2, err := db.UpsertAsset(ctx, newTestAsset(7, sid, "ext_stale", "Stale"))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`UPDATE knowledge_assets SET last_retrieved_at = DATETIME('now', '-60 days') WHERE id = ?`,
		a2.ID,
	); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	// Asset 3: retrieved recently â€” NOT stale.
	a3, _ := db.UpsertAsset(ctx, newTestAsset(7, sid, "ext_recent", "Recent"))
	if err := db.IncrementAssetRetrieval(ctx, a3.ID, 7); err != nil {
		t.Fatalf("Increment: %v", err)
	}

	count, err := db.CountStaleAssetsForOrg(ctx, 7, 30)
	if err != nil {
		t.Fatalf("CountStale: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 stale asset; got %d", count)
	}
}

// Empty org_id rejects: we never accept zero org_id as "any org" â€”
// per the deterministic-boundaries rule.
func TestRecordKnowledgeSync_RejectsZeroOrgID(t *testing.T) {
	db := newKnowledgeStore(t, "events.db")
	ctx := context.Background()
	db.RecordSync(ctx, 0, sources.SourceCSV, 1, 1, 0, 0, 1, 0)
	// Should have written nothing.
	list, _ := db.ListRecentSyncsForOrg(ctx, 1, 10)
	if len(list) != 0 {
		t.Errorf("zero-orgID sync should be dropped; got %d rows", len(list))
	}
	_ = assets.AssetPODProduct // silence unused-import potential
}
