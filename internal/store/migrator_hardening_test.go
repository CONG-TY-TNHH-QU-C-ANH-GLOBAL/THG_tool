// Domain: infra
package store

import (
	"context"
	"path/filepath"
	"testing"
)

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
	if err := db.applyMigration(ctx, db.db, Migration{Version: 9001, Name: "good", SQL: `CREATE TABLE tx_probe (x INT)`}); err != nil {
		t.Fatalf("good migration: %v", err)
	}
	if !db.tableExists(ctx, "tx_probe") {
		t.Fatal("good migration did not create tx_probe")
	}
	applied, _ := db.appliedMigrationVersions(ctx, db.db)
	if _, ok := applied[9001]; !ok {
		t.Fatal("good migration version 9001 not recorded")
	}

	// Failing migration (2nd statement is invalid) → whole tx rolls back: the
	// first statement's table must NOT persist and the version must NOT record.
	failSQL := `CREATE TABLE should_not_persist (x INT); THIS IS NOT VALID SQL;`
	if err := db.applyMigration(ctx, db.db, Migration{Version: 9002, Name: "bad", SQL: failSQL}); err == nil {
		t.Fatal("failing migration should return an error")
	}
	if db.tableExists(ctx, "should_not_persist") {
		t.Fatal("failing migration was NOT rolled back (partial table persisted)")
	}
	applied, _ = db.appliedMigrationVersions(ctx, db.db)
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
