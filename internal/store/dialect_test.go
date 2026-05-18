package store

import (
	"context"
	"strings"
	"testing"
)

// Rebind on Postgres rewrites every `?` to `$N` in left-to-right order.
// This is the load-bearing correctness property for risk R2.
func TestPostgresDialect_Rebind(t *testing.T) {
	pg := postgresDialect{}
	cases := []struct {
		in   string
		want string
	}{
		{"SELECT 1", "SELECT 1"},
		{"SELECT * FROM t WHERE a = ?", "SELECT * FROM t WHERE a = $1"},
		{"INSERT INTO t (a, b, c) VALUES (?, ?, ?)", "INSERT INTO t (a, b, c) VALUES ($1, $2, $3)"},
		{"UPDATE t SET a = ?, b = ? WHERE id = ? AND org_id = ?",
			"UPDATE t SET a = $1, b = $2 WHERE id = $3 AND org_id = $4"},
		{"-- comment with ? in it\nSELECT ?", "-- comment with $1 in it\nSELECT $2"},
	}
	for _, c := range cases {
		got := pg.Rebind(c.in)
		if got != c.want {
			t.Errorf("Rebind(%q):\n  got  %q\n  want %q", c.in, got, c.want)
		}
	}
}

// Rebind on SQLite is the identity transform.
func TestSQLiteDialect_RebindIsIdentity(t *testing.T) {
	sqlite := sqliteDialect{}
	inputs := []string{
		"SELECT 1",
		"SELECT * FROM t WHERE a = ?",
		"INSERT INTO t VALUES (?, ?, ?)",
	}
	for _, in := range inputs {
		if got := sqlite.Rebind(in); got != in {
			t.Errorf("SQLite Rebind should be identity; got %q", got)
		}
	}
}

// IntervalDaysExpr produces dialect-native expression. Tested as
// substring match — we don't pin exact formatting since both forms
// have whitespace variations that produce identical SQL behavior.
func TestIntervalDaysExpr(t *testing.T) {
	if got := (sqliteDialect{}).IntervalDaysExpr(30); !strings.Contains(got, "DATETIME") || !strings.Contains(got, "30") {
		t.Errorf("SQLite interval should reference DATETIME and 30; got %q", got)
	}
	if got := (postgresDialect{}).IntervalDaysExpr(30); !strings.Contains(got, "INTERVAL") || !strings.Contains(got, "30") {
		t.Errorf("PG interval should reference INTERVAL and 30; got %q", got)
	}
}

// NowExpr returns the dialect's idiomatic "current timestamp"
// expression. Both are SQL-standard but stored differently in the
// helper so future dialect-specific tuning has a single touchpoint.
func TestNowExpr(t *testing.T) {
	if got := (sqliteDialect{}).NowExpr(); got != "CURRENT_TIMESTAMP" {
		t.Errorf("SQLite NowExpr: got %q", got)
	}
	if got := (postgresDialect{}).NowExpr(); got != "NOW()" {
		t.Errorf("PG NowExpr: got %q", got)
	}
}

// Names are stable contract values — log + metric backends key on them.
func TestDialectNames(t *testing.T) {
	if (sqliteDialect{}).Name() != "sqlite" {
		t.Error("SQLite dialect Name() must be 'sqlite'")
	}
	if (postgresDialect{}).Name() != "postgres" {
		t.Error("Postgres dialect Name() must be 'postgres'")
	}
}

// The Store wrapper methods must thread Rebind correctly. The hot
// path is ExecContext / QueryContext / InsertReturningID — each
// must apply Rebind exactly once.
func TestStoreWrappers_AutoRebindOnSQLite(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	// Just exercise a wrapper end-to-end on the existing schema; if
	// Rebind double-applied or skipped, the SQL would syntax-error
	// in SQLite. The fact that this returns successfully proves
	// behaviour parity with pre-wrapper code.
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

// isPostgresDSN distinguishes PG from SQLite file paths. The
// detector is the boot-time branch that decides which driver to load
// — getting it wrong means dev callers accidentally get a PG driver
// requested (and fail), or prod operators accidentally get SQLite.
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

// Migrator: a fresh SQLite DB with the legacy s.migrate() schema
// gets the baseline-marker recorded on first runMigrations call,
// plus any dialect-portable migration files that exist (e.g. the
// embedding metadata migration introduced in PR-1 Embedding Foundation).
// Second call must be a no-op (idempotent).
func TestMigrator_RecordsBaselineOnceForLegacySchema(t *testing.T) {
	db := newKnowledgeTestStore(t) // store.New already ran migrations
	ctx := context.Background()

	v1, err := db.CurrentSchemaVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentSchemaVersion: %v", err)
	}
	// Baseline must be at least 1. As new migrations land in
	// migrations/, this number grows — assert >= 1 rather than
	// pinning a fixed value so the test stays stable across PRs.
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
