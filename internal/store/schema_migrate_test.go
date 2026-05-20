// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestMigrate_FullRunCreatesAllExpectedTables proves migrate() builds
// every table the runtime + later migration files depend on. The
// concrete signal: knowledge_assets must exist after a fresh migrate,
// because migration 0002 (add_embedding_metadata) ALTERs it. Production
// CD failed on 2026-05-19 when a fast-path skip left knowledge_assets
// uncreated, then migration 0002 hit "no such table: knowledge_assets".
func TestMigrate_FullRunCreatesAllExpectedTables(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	for _, table := range []string{
		"groups",            // v1, first table
		"knowledge_assets",  // expected by migration 0002
		"knowledge_sources", // baseline marker probe
		"knowledge_feedback", // last table written by migrate()
	} {
		if !db.tableExists(context.Background(), table) {
			t.Errorf("table %q missing after fresh migrate()", table)
		}
	}
}

// TestMigrate_FastPathSkipsOnlyAtCurrentVersion is the regression
// guard against the 2026-05-19 production CD failure. Production
// already had `groups` (long-lived table) but NOT `knowledge_assets`
// (added later to migrate); an earlier fast-path that probed `groups`
// skipped migrate, leaving knowledge_assets uncreated, and migration
// 0002 then failed with "no such table: knowledge_assets". The
// marker-based fast-path must NOT skip when the marker is absent.
//
// We simulate the production state by running a full migrate, then
// dropping the marker + a "newly-added" table. Reopening MUST detect
// the missing marker and re-run migrate(), recreating the table.
func TestMigrate_FastPathSkipsOnlyAtCurrentVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// Pass 1: clean bootstrap.
	{
		db, err := New(path)
		if err != nil {
			t.Fatalf("first New: %v", err)
		}
		if !db.schemaAlreadyApplied() {
			db.Close()
			t.Fatal("marker missing right after a fresh migrate()")
		}
		// Simulate a stale production DB: drop the marker and one of
		// the tables added later to migrate(). The fast-path MUST re-run
		// on next open and recreate the dropped table.
		if _, err := db.db.Exec(`DROP TABLE knowledge_assets`); err != nil {
			db.Close()
			t.Fatalf("drop knowledge_assets: %v", err)
		}
		if _, err := db.db.Exec(`DELETE FROM _schema_bootstrap_marker`); err != nil {
			db.Close()
			t.Fatalf("clear marker: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}

	// Pass 2: reopen the degraded DB. migrate() must NOT take the
	// fast-path and must recreate knowledge_assets.
	db, err := New(path)
	if err != nil {
		t.Fatalf("reopen legacy DB: %v", err)
	}
	defer db.Close()

	if !db.tableExists(context.Background(), "knowledge_assets") {
		t.Fatal("migrate() fast-path skipped despite missing marker; knowledge_assets not recreated — this would break migration 0002 in production")
	}
	if !db.schemaAlreadyApplied() {
		t.Error("marker not written after the recovery migrate() run")
	}
}
