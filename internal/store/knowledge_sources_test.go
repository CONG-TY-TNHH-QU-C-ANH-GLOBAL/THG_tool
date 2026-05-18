package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

func newKnowledgeTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestSource(orgID int64, label string, typ sources.SourceType) *sources.Source {
	return &sources.Source{
		OrgID:            orgID,
		Type:             typ,
		Label:            label,
		ConnectionConfig: json.RawMessage(`{"endpoint":"https://example/api"}`),
		SyncPolicy:       sources.SyncManual,
		Health: sources.Health{
			Status: sources.HealthHealthy,
		},
	}
}

// Upsert insert path persists every field and round-trips through
// Get. This is the basic happy-path that proves the schema + scanner
// agree on column order.
func TestUpsertKnowledgeSource_RoundTrip(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	src := newTestSource(1, "Main Catalog", sources.SourceShopify)
	src.SyncPolicy = sources.SyncDaily

	saved, err := db.UpsertKnowledgeSource(ctx, src)
	if err != nil {
		t.Fatalf("UpsertKnowledgeSource: %v", err)
	}
	if saved.ID == 0 {
		t.Fatal("saved.ID should be non-zero after insert")
	}
	if saved.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set by SQL CURRENT_TIMESTAMP")
	}

	got, err := db.GetKnowledgeSource(ctx, saved.ID, 1)
	if err != nil {
		t.Fatalf("GetKnowledgeSource: %v", err)
	}
	if got.Label != "Main Catalog" {
		t.Errorf("label round-trip: got %q want %q", got.Label, "Main Catalog")
	}
	if got.Type != sources.SourceShopify {
		t.Errorf("type round-trip: got %q want %q", got.Type, sources.SourceShopify)
	}
	if got.SyncPolicy != sources.SyncDaily {
		t.Errorf("sync_policy round-trip: got %q want %q", got.SyncPolicy, sources.SyncDaily)
	}
	if got.Health.LastSyncAt != nil {
		t.Errorf("LastSyncAt should be nil for a never-synced source, got %v", got.Health.LastSyncAt)
	}
}

// Cross-org leak guard: GetKnowledgeSource against a foreign org must
// return sql.ErrNoRows, NOT a permission-denied error. This makes
// foreign-org rows observably indistinguishable from missing rows —
// the design-doc invariant #1.
func TestGetKnowledgeSource_ForeignOrgIsNotFound(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	saved, err := db.UpsertKnowledgeSource(ctx, newTestSource(1, "Org-1 Source", sources.SourceCSV))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Same ID, different org — must look like "not found."
	_, err = db.GetKnowledgeSource(ctx, saved.ID, 2)
	if err != sql.ErrNoRows {
		t.Errorf("foreign org read should return sql.ErrNoRows; got %v", err)
	}
}

// Cross-org leak guard: UpsertKnowledgeSource UPDATE path against a
// foreign org's row must NOT mutate it, even if the caller passes the
// correct ID. The WHERE org_id = ? clause is the gate.
func TestUpsertKnowledgeSource_CannotUpdateForeignOrg(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	saved, err := db.UpsertKnowledgeSource(ctx, newTestSource(1, "Org-1 Original", sources.SourceCSV))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Try to update org-1's row claiming to be org-2.
	hostile := *saved
	hostile.OrgID = 2
	hostile.Label = "HIJACKED"
	_, err = db.UpsertKnowledgeSource(ctx, &hostile)
	if err != sql.ErrNoRows {
		t.Errorf("foreign-org update should return sql.ErrNoRows; got %v", err)
	}

	// The original row must be untouched.
	again, err := db.GetKnowledgeSource(ctx, saved.ID, 1)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if again.Label != "Org-1 Original" {
		t.Errorf("foreign-org update bled through: label is now %q", again.Label)
	}
}

// Cross-org leak guard: DeleteKnowledgeSourceForOrg against a foreign
// org's source must NOT delete it.
func TestDeleteKnowledgeSource_CannotDeleteForeignOrg(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	saved, err := db.UpsertKnowledgeSource(ctx, newTestSource(1, "Org-1", sources.SourceWebsite))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	_, err = db.DeleteKnowledgeSourceForOrg(ctx, saved.ID, 2)
	if err != sql.ErrNoRows {
		t.Errorf("foreign-org delete should return sql.ErrNoRows; got %v", err)
	}

	// Org-1's row survives.
	if _, err := db.GetKnowledgeSource(ctx, saved.ID, 1); err != nil {
		t.Errorf("org-1 row should survive foreign-org delete attempt; got %v", err)
	}
}

// ListKnowledgeSourcesForOrg returns ONLY rows owned by orgID. Two
// orgs with similarly-labelled sources must not see each other.
func TestListKnowledgeSourcesForOrg_TenantScope(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	if _, err := db.UpsertKnowledgeSource(ctx, newTestSource(1, "Org-1 Shopify", sources.SourceShopify)); err != nil {
		t.Fatalf("upsert org-1: %v", err)
	}
	if _, err := db.UpsertKnowledgeSource(ctx, newTestSource(1, "Org-1 CSV", sources.SourceCSV)); err != nil {
		t.Fatalf("upsert org-1 #2: %v", err)
	}
	if _, err := db.UpsertKnowledgeSource(ctx, newTestSource(2, "Org-2 Shopify", sources.SourceShopify)); err != nil {
		t.Fatalf("upsert org-2: %v", err)
	}

	list1, err := db.ListKnowledgeSourcesForOrg(ctx, 1, sources.ListFilter{})
	if err != nil {
		t.Fatalf("list org-1: %v", err)
	}
	if len(list1) != 2 {
		t.Errorf("org-1 should see 2 sources, got %d", len(list1))
	}
	for _, src := range list1 {
		if src.OrgID != 1 {
			t.Errorf("org-1 list leaked org_id=%d", src.OrgID)
		}
	}

	list2, err := db.ListKnowledgeSourcesForOrg(ctx, 2, sources.ListFilter{})
	if err != nil {
		t.Fatalf("list org-2: %v", err)
	}
	if len(list2) != 1 {
		t.Errorf("org-2 should see 1 source, got %d", len(list2))
	}
}

// UpdateKnowledgeSourceHealth must not be reachable via UpsertKnowledgeSource.
// Even if the operator sends a struct with Health.Status="error", the
// Upsert UPDATE path leaves health alone — so an operator editing the
// label cannot accidentally mark the source healthy.
func TestUpsertKnowledgeSource_DoesNotTouchHealth(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	saved, err := db.UpsertKnowledgeSource(ctx, newTestSource(1, "Original", sources.SourceCSV))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// Ingestor reports a real failure.
	at := time.Now()
	if err := db.UpdateKnowledgeSourceHealth(ctx, saved.ID, 1,
		sources.Health{Status: sources.HealthError, Message: "401 Unauthorized", LastSyncAt: &at},
		0,
	); err != nil {
		t.Fatalf("UpdateHealth: %v", err)
	}

	// Operator now edits the label, sending Health.Status=healthy in
	// the payload (e.g. because the frontend filled the struct from a
	// stale GET response). The Upsert UPDATE path must IGNORE that.
	edit := *saved
	edit.Label = "Renamed"
	edit.Health.Status = sources.HealthHealthy
	edit.Health.Message = ""
	if _, err := db.UpsertKnowledgeSource(ctx, &edit); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}

	got, err := db.GetKnowledgeSource(ctx, saved.ID, 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Label != "Renamed" {
		t.Errorf("label edit did not persist: %q", got.Label)
	}
	if got.Health.Status != sources.HealthError {
		t.Errorf("health was clobbered by Upsert; got %q want %q",
			got.Health.Status, sources.HealthError)
	}
	if got.Health.Message != "401 Unauthorized" {
		t.Errorf("health message was clobbered: %q", got.Health.Message)
	}
}

// Validate is the boundary gate: malformed sources must be rejected
// at the repository, not silently coerced.
func TestUpsertKnowledgeSource_RejectsInvalid(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	cases := []struct {
		name string
		mut  func(*sources.Source)
	}{
		{"zero org_id", func(s *sources.Source) { s.OrgID = 0 }},
		{"unknown type", func(s *sources.Source) { s.Type = "totally-bogus" }},
		{"empty label", func(s *sources.Source) { s.Label = "   " }},
		{"malformed connection_config", func(s *sources.Source) {
			s.ConnectionConfig = json.RawMessage(`{not json`)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := newTestSource(1, "ok", sources.SourceCSV)
			tc.mut(src)
			if _, err := db.UpsertKnowledgeSource(ctx, src); err == nil {
				t.Errorf("expected validation error for %s; got nil", tc.name)
			}
		})
	}
}
