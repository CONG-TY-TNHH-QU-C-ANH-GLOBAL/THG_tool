package knowledge_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/thg/scraper/internal/store/knowledge"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

func newTestAsset(orgID, sourceID int64, externalID, title string) *assets.Asset {
	return &assets.Asset{
		OrgID:       orgID,
		SourceID:    sourceID,
		ExternalID:  externalID,
		Type:        assets.AssetPODProduct,
		Title:       title,
		Description: "test description",
		Tags:        []string{"cat", "tee", "pet"},
		Payload:     json.RawMessage(`{"price":"$18.50"}`),
		State:       assets.StatePending,
	}
}

// Helper: stand up a source so assets have a valid FK target.
func mustSetupSource(t *testing.T, db *knowledge.Store, orgID int64) int64 {
	t.Helper()
	src, err := db.UpsertSource(context.Background(),
		newTestSource(orgID, "test source", sources.SourceCSV))
	if err != nil {
		t.Fatalf("setup source: %v", err)
	}
	return src.ID
}

// Happy-path round trip â€” proves schema + scanner agree on column order.
func TestUpsertKnowledgeAsset_RoundTrip(t *testing.T) {
	db := newKnowledgeStore(t, "assets.db")
	ctx := context.Background()
	sid := mustSetupSource(t, db, 1)

	a := newTestAsset(1, sid, "shopify_gid_001", "Custom Cat Tee")
	saved, err := db.UpsertAsset(ctx, a)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if saved.ID == 0 {
		t.Error("ID should be non-zero after insert")
	}
	if len(saved.Tags) != 3 {
		t.Errorf("tags lost in round-trip: got %v", saved.Tags)
	}
	if saved.State != assets.StatePending {
		t.Errorf("state round-trip: got %q want %q", saved.State, assets.StatePending)
	}
	if saved.Metrics.LastRetrievedAt != nil {
		t.Error("LastRetrievedAt should be nil for never-retrieved asset")
	}
}

// Invariant 1: cross-org leak guard for GET.
func TestGetKnowledgeAsset_ForeignOrgIsNotFound(t *testing.T) {
	db := newKnowledgeStore(t, "assets.db")
	ctx := context.Background()

	sid := mustSetupSource(t, db, 1)
	saved, err := db.UpsertAsset(ctx, newTestAsset(1, sid, "ext_1", "Org-1 Asset"))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	_, err = db.GetAsset(ctx, saved.ID, 2)
	if err != sql.ErrNoRows {
		t.Errorf("foreign-org GET should return sql.ErrNoRows; got %v", err)
	}
}

// Invariant 1: a caller cannot smuggle an asset under a source_id
// that belongs to another tenant.
func TestUpsertKnowledgeAsset_RejectsForeignSourceID(t *testing.T) {
	db := newKnowledgeStore(t, "assets.db")
	ctx := context.Background()

	sid1 := mustSetupSource(t, db, 1)

	// Org-2 tries to create an asset citing org-1's source_id.
	hostile := newTestAsset(2, sid1, "ext_hostile", "Cross-tenant smuggle")
	_, err := db.UpsertAsset(ctx, hostile)
	if err == nil {
		t.Fatal("expected error when source_id belongs to a different org; got nil")
	}
}

// Invariant 1: operator setters refuse foreign-org rows.
func TestSetKnowledgeAssetState_ForeignOrgIsIgnored(t *testing.T) {
	db := newKnowledgeStore(t, "assets.db")
	ctx := context.Background()

	sid := mustSetupSource(t, db, 1)
	saved, err := db.UpsertAsset(ctx, newTestAsset(1, sid, "ext_1", "Org-1"))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Org-2 attempts to hide org-1's asset.
	if err := db.SetAssetState(ctx, saved.ID, 2, assets.StateHidden); err != sql.ErrNoRows {
		t.Errorf("foreign-org state change should return sql.ErrNoRows; got %v", err)
	}

	// Org-1's asset is unchanged.
	got, err := db.GetAsset(ctx, saved.ID, 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != assets.StatePending {
		t.Errorf("state was changed by foreign-org caller: got %q", got.State)
	}
}

// Invariant 2: idempotent ingest. Re-Upserting the same external_id
// UPDATES the row instead of creating a duplicate.
func TestUpsertKnowledgeAsset_IdempotentReSync(t *testing.T) {
	db := newKnowledgeStore(t, "assets.db")
	ctx := context.Background()
	sid := mustSetupSource(t, db, 1)

	first, err := db.UpsertAsset(ctx, newTestAsset(1, sid, "stable_gid_42", "Original Title"))
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Second upsert with the same external_id and a new title.
	second := newTestAsset(1, sid, "stable_gid_42", "Updated Title")
	second.Description = "fresh description"
	second.Tags = []string{"updated"}
	got, err := db.UpsertAsset(ctx, second)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if got.ID != first.ID {
		t.Errorf("idempotent ingest produced a NEW row: first=%d second=%d", first.ID, got.ID)
	}
	if got.Title != "Updated Title" {
		t.Errorf("title should have been updated; got %q", got.Title)
	}
	if got.Description != "fresh description" {
		t.Errorf("description should have been updated; got %q", got.Description)
	}

	// And there is exactly one row for the (org, source, external_id) tuple.
	list, err := db.ListAssetsForOrg(ctx, 1, assets.ListFilter{SourceID: sid})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("idempotent ingest produced %d rows; want 1", len(list))
	}
}

// Invariant 3: operator state survives a re-sync. If an operator pins
// an asset, then the ingestor re-syncs the source, the pinned flag
// must remain. Same for `state` and `boost`.
//
// This is the load-bearing test from the design doc.
func TestUpsertKnowledgeAsset_OperatorStateSurvivesReSync(t *testing.T) {
	db := newKnowledgeStore(t, "assets.db")
	ctx := context.Background()
	sid := mustSetupSource(t, db, 1)

	original, err := db.UpsertAsset(ctx, newTestAsset(1, sid, "stable_001", "v1"))
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	// Operator approves, pins, and boosts.
	if err := db.SetAssetState(ctx, original.ID, 1, assets.StateApproved); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	if err := db.SetAssetPinned(ctx, original.ID, 1, true); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}
	if err := db.SetAssetBoost(ctx, original.ID, 1, 75); err != nil {
		t.Fatalf("SetBoost: %v", err)
	}

	// Ingestor re-syncs. The fresh asset carries default operator
	// fields (state=pending, pinned=false, boost=0). These MUST NOT
	// overwrite the operator's choices.
	resync := newTestAsset(1, sid, "stable_001", "v2 â€” updated title")
	resync.Description = "new description from ingestor"
	if _, err := db.UpsertAsset(ctx, resync); err != nil {
		t.Fatalf("re-ingest: %v", err)
	}

	got, err := db.GetAsset(ctx, original.ID, 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Ingestor-controlled â€” updated.
	if got.Title != "v2 â€” updated title" {
		t.Errorf("title should be updated by re-ingest; got %q", got.Title)
	}
	if got.Description != "new description from ingestor" {
		t.Errorf("description should be updated by re-ingest; got %q", got.Description)
	}
	// Operator-controlled â€” survived.
	if got.State != assets.StateApproved {
		t.Errorf("state regressed during re-ingest: got %q want %q", got.State, assets.StateApproved)
	}
	if !got.Pinned {
		t.Error("pinned was cleared by re-ingest")
	}
	if got.Boost != 75 {
		t.Errorf("boost was clobbered by re-ingest: got %d want 75", got.Boost)
	}
}

// Invariant 3 (continued): re-sync must NOT reset retrieval metrics
// either. The metrics counter increments through a separate code
// path and must persist across ingest cycles.
func TestUpsertKnowledgeAsset_MetricsSurviveReSync(t *testing.T) {
	db := newKnowledgeStore(t, "assets.db")
	ctx := context.Background()
	sid := mustSetupSource(t, db, 1)

	a, err := db.UpsertAsset(ctx, newTestAsset(1, sid, "stable_m", "with metrics"))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Simulate three retrievals.
	for range 3 {
		if err := db.IncrementAssetRetrieval(ctx, a.ID, 1); err != nil {
			t.Fatalf("Increment: %v", err)
		}
	}

	// Re-ingest.
	if _, err := db.UpsertAsset(ctx, newTestAsset(1, sid, "stable_m", "with metrics v2")); err != nil {
		t.Fatalf("re-ingest: %v", err)
	}

	got, err := db.GetAsset(ctx, a.ID, 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metrics.Retrievals30d != 3 {
		t.Errorf("Retrievals30d reset by re-ingest; got %d want 3", got.Metrics.Retrievals30d)
	}
	if got.Metrics.LastRetrievedAt == nil {
		t.Error("LastRetrievedAt was cleared by re-ingest")
	}
}

// ListKnowledgeAssetsForOrg filters by state correctly. The Product
// Explorer panel relies on this; if it regresses, hidden assets show
// up in the default view.
func TestListKnowledgeAssetsForOrg_StateFilter(t *testing.T) {
	db := newKnowledgeStore(t, "assets.db")
	ctx := context.Background()
	sid := mustSetupSource(t, db, 1)

	for i, st := range []assets.AssetState{
		assets.StatePending,
		assets.StateApproved,
		assets.StateApproved,
		assets.StateHidden,
	} {
		a := newTestAsset(1, sid, "ext_"+itoa(i), "row "+itoa(i))
		a.State = st
		if _, err := db.UpsertAsset(ctx, a); err != nil {
			t.Fatalf("Upsert #%d: %v", i, err)
		}
	}

	// Empty filter returns everything (Product Explorer's "All" tab).
	all, err := db.ListAssetsForOrg(ctx, 1, assets.ListFilter{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 assets in 'all' view; got %d", len(all))
	}

	// Approved-only â€” what the retrieval engine reads.
	approved, err := db.ListAssetsForOrg(ctx, 1, assets.ListFilter{
		States: []assets.AssetState{assets.StateApproved},
	})
	if err != nil {
		t.Fatalf("list approved: %v", err)
	}
	if len(approved) != 2 {
		t.Errorf("expected 2 approved assets; got %d", len(approved))
	}
	for _, a := range approved {
		if a.State != assets.StateApproved {
			t.Errorf("approved-filter leaked state=%q", a.State)
		}
	}
}

// Cascade delete: removing a source removes its assets in the same
// transaction. Asserts the count too â€” operators need to know the
// blast radius.
func TestDeleteKnowledgeSourceForOrg_CascadesAssets(t *testing.T) {
	db := newKnowledgeStore(t, "assets.db")
	ctx := context.Background()
	sid := mustSetupSource(t, db, 1)

	for i := range 4 {
		if _, err := db.UpsertAsset(ctx, newTestAsset(1, sid, "ext_"+itoa(i), "row "+itoa(i))); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	deleted, err := db.DeleteSourceForOrg(ctx, sid, 1)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if deleted != 4 {
		t.Errorf("expected 4 cascaded assets; got %d", deleted)
	}
	left, err := db.ListAssetsForOrg(ctx, 1, assets.ListFilter{})
	if err != nil {
		t.Fatalf("post-delete list: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("assets survived cascade delete: %d remaining", len(left))
	}
}

// Boost is clamped at the boundary, not silently mis-stored.
func TestSetKnowledgeAssetBoost_Clamps(t *testing.T) {
	db := newKnowledgeStore(t, "assets.db")
	ctx := context.Background()
	sid := mustSetupSource(t, db, 1)
	a, err := db.UpsertAsset(ctx, newTestAsset(1, sid, "ext_b", "boost test"))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	cases := []struct {
		in   int
		want int
	}{
		{-50, 0},
		{0, 0},
		{50, 50},
		{100, 100},
		{200, 100},
	}
	for _, c := range cases {
		if err := db.SetAssetBoost(ctx, a.ID, 1, c.in); err != nil {
			t.Fatalf("SetBoost(%d): %v", c.in, err)
		}
		got, _ := db.GetAsset(ctx, a.ID, 1)
		if got.Boost != c.want {
			t.Errorf("SetBoost(%d): persisted %d, want %d (clamped)", c.in, got.Boost, c.want)
		}
	}
}

// Search filter does a case-insensitive substring on title + tags.
// This is the LIKE-based naive search the MVP uses; the retrieval
// engine port will eventually replace it with semantic search.
func TestListKnowledgeAssetsForOrg_SearchFilter(t *testing.T) {
	db := newKnowledgeStore(t, "assets.db")
	ctx := context.Background()
	sid := mustSetupSource(t, db, 1)

	rows := []struct {
		ext, title string
		tags       []string
	}{
		{"e1", "Cat Tee Premium", []string{"cat", "tee"}},
		{"e2", "Dog Mug", []string{"dog", "mug"}},
		{"e3", "Pet Hoodie", []string{"cat", "hoodie"}},
	}
	for _, r := range rows {
		a := newTestAsset(1, sid, r.ext, r.title)
		a.Tags = r.tags
		if _, err := db.UpsertAsset(ctx, a); err != nil {
			t.Fatalf("seed %s: %v", r.ext, err)
		}
	}

	hits, err := db.ListAssetsForOrg(ctx, 1, assets.ListFilter{SearchQ: "cat"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// "Cat Tee Premium" matches on title; "Pet Hoodie" matches on tags.
	if len(hits) != 2 {
		t.Errorf("search 'cat' should match 2 rows (title + tags); got %d", len(hits))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string('0'+rune(n%10)) + s
		n /= 10
	}
	return s
}
