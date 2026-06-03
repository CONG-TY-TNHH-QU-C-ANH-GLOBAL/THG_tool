// Domain: infra
package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// TestBenignMigrationErr pins which legacy-baseline statement errors are
// tolerated (idempotency) vs which abort the bootstrap (genuine failures).
func TestBenignMigrationErr(t *testing.T) {
	benign := []string{
		"duplicate column name: x",
		"table foo already exists",
		"index idx_y already exists",
		"no such column: status",
	}
	for _, m := range benign {
		if !benignMigrationErr(errors.New(m)) {
			t.Errorf("expected benign: %q", m)
		}
	}
	genuine := []string{
		"near \"SELCT\": syntax error",
		"no such table: accounts",
		"UNIQUE constraint failed: accounts.id",
		"disk I/O error",
	}
	for _, m := range genuine {
		if benignMigrationErr(errors.New(m)) {
			t.Errorf("expected genuine (must abort): %q", m)
		}
	}
	if !benignMigrationErr(nil) {
		t.Error("nil must be benign")
	}
}

// TestApplyMigration_Atomic verifies the runner applies a migration body and its
// version record ATOMICALLY, and that a failing migration leaves NOTHING behind
// (no half-applied table, no version row) — the production-grade guarantee.
func TestApplyMigration_Atomic(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "runner.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	// Good migration → table created AND version recorded.
	if err := db.applyMigration(ctx, Migration{Version: 9001, Name: "good", SQL: `CREATE TABLE tx_probe (x INT)`}); err != nil {
		t.Fatalf("good migration: %v", err)
	}
	if !db.tableExists(ctx, "tx_probe") {
		t.Fatal("good migration did not create tx_probe")
	}
	applied, _ := db.appliedMigrationVersions(ctx)
	if _, ok := applied[9001]; !ok {
		t.Fatal("good migration version 9001 not recorded")
	}

	// Failing migration (2nd statement is invalid) → whole tx rolls back: the
	// first statement's table must NOT persist and the version must NOT record.
	failSQL := `CREATE TABLE should_not_persist (x INT); THIS IS NOT VALID SQL;`
	if err := db.applyMigration(ctx, Migration{Version: 9002, Name: "bad", SQL: failSQL}); err == nil {
		t.Fatal("failing migration should return an error")
	}
	if db.tableExists(ctx, "should_not_persist") {
		t.Fatal("failing migration was NOT rolled back (partial table persisted)")
	}
	applied, _ = db.appliedMigrationVersions(ctx)
	if _, ok := applied[9002]; ok {
		t.Fatal("failing migration version must NOT be recorded")
	}
}

// TestMigrationNotxOptOut verifies the `-- migrate:notx` escape hatch detection.
func TestMigrationNotxOptOut(t *testing.T) {
	if !migrationOptsOutOfTx("-- migrate:notx\nCREATE INDEX CONCURRENTLY ...") {
		t.Error("should detect notx directive")
	}
	if !migrationOptsOutOfTx("-- a comment\n-- migrate:notx\nSELECT 1") {
		t.Error("should detect notx after other comments")
	}
	if migrationOptsOutOfTx("CREATE TABLE t(x INT)") {
		t.Error("plain SQL must not opt out")
	}
	if migrationOptsOutOfTx("-- just a note\nCREATE TABLE t(x INT)") {
		t.Error("unrelated comment must not opt out")
	}
}
