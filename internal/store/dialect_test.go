// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"
)

// The Store wrapper methods must thread Rebind correctly. The hot
// path is ExecContext / QueryContext / InsertReturningID — each must
// apply Rebind exactly once.
func TestStoreWrappers_AutoRebindOnSQLite(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	// Just exercise a wrapper end-to-end on the existing schema; if
	// Rebind double-applied or skipped, the SQL would syntax-error in
	// SQLite. The fact that this returns successfully proves behaviour
	// parity with pre-wrapper code.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO knowledge_sources
			(org_id, type, label, sync_policy, health_status)
		 VALUES (?, ?, ?, ?, ?)`,
		1, "csv", "test", "manual", "healthy"); err != nil {
		t.Fatalf("ExecContext via wrapper: %v", err)
	}
	var n int
	row := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM knowledge_sources WHERE org_id = ?`, 1)
	if err := row.Scan(&n); err != nil {
		t.Fatalf("QueryRowContext via wrapper: %v", err)
	}
	if n != 1 {
		t.Errorf("wrapper round-trip failed; got %d row(s)", n)
	}
}

// InsertReturningID on SQLite returns the new row's ID via RETURNING.
// Asserts that SQLite >= 3.35 (required for RETURNING) is what the
// embedded modernc/sqlite driver actually provides — failures here
// surface as a clear "near RETURNING: syntax error" not a silent
// wrong-id bug.
func TestStore_InsertReturningID_SQLite(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	id, err := db.InsertReturningID(ctx,
		`INSERT INTO knowledge_sources
			(org_id, type, label, sync_policy, health_status)
		 VALUES (?, ?, ?, ?, ?) RETURNING id`,
		1, "csv", "test", "manual", "healthy")
	if err != nil {
		t.Fatalf("InsertReturningID: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive id; got %d", id)
	}
}

// isPostgresDSN distinguishes PG from SQLite file paths. The detector
// is the boot-time branch that decides which driver to load — getting
// it wrong means dev callers accidentally get a PG driver requested
// (and fail), or prod operators accidentally get SQLite.
func TestIsPostgresDSN(t *testing.T) {
	pgCases := []string{
		"postgres://user:pass@host/db",
		"postgresql://host/db",
		"POSTGRES://user@host/db",         // case-insensitive scheme
		"host=localhost port=5432 user=x", // libpq keyword form
	}
	for _, c := range pgCases {
		if !isPostgresDSN(c) {
			t.Errorf("isPostgresDSN(%q) = false; want true", c)
		}
	}
	notPG := []string{
		"data/store.db",
		"/var/lib/store/db.sqlite",
		"./relative/path",
		"",
		"sqlite://file",
	}
	for _, c := range notPG {
		if isPostgresDSN(c) {
			t.Errorf("isPostgresDSN(%q) = true; want false", c)
		}
	}
}

// Migrator: a fresh SQLite DB with the legacy s.migrate() schema gets
// the baseline-marker recorded on first runMigrations call, plus any
// dialect-portable migration files that exist (e.g. the embedding
// metadata migration introduced in PR-1 Embedding Foundation). Second
// call must be a no-op (idempotent).
func TestMigrator_RecordsBaselineOnceForLegacySchema(t *testing.T) {
	db := newKnowledgeTestStore(t) // store.New already ran migrations
	ctx := context.Background()

	v1, err := db.CurrentSchemaVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentSchemaVersion: %v", err)
	}
	// Baseline must be at least 1. As new migrations land in
	// migrations/, this number grows — assert >= 1 rather than pinning
	// a fixed value so the test stays stable across PRs.
	if v1 < 1 {
		t.Errorf("baseline should be recorded (version >= 1); got %d", v1)
	}

	// Re-running migrations must not double-record or change the version.
	if err := db.runMigrations(ctx); err != nil {
		t.Fatalf("second runMigrations: %v", err)
	}
	v2, _ := db.CurrentSchemaVersion(ctx)
	if v2 != v1 {
		t.Errorf("re-run should not bump version; got %d, want %d", v2, v1)
	}
}
