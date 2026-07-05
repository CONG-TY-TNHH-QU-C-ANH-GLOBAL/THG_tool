// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"os"
	"testing"
)

// TestRealPostgresApply is the "real Postgres apply" promotion that
// internal/store/migrations/README.md gates on (see its "statically
// validated only" note). It proves, against an ACTUAL PostgreSQL database,
// that:
//
//  1. every platform-baseline migration (0100-0110) applies via
//     store.New's normal boot path (layer 1, runMigrations), and
//  2. the layer-2 bootstrap (sessions.Migrate / app.Migrate) no longer runs
//     SQLite-only DDL against Postgres — the historical PG boot blocker.
//
// Skipped (not failed) unless POSTGRES_PLATFORM_TEST_DSN is set, so
// `go test ./...` stays green without a database — see
// internal/store/postgres/README.md for the sibling POSTGRES_TEST_DSN
// convention. A DEDICATED env var/database is used here, distinct from
// internal/store/postgres's POSTGRES_TEST_DSN: that package drops and
// recreates outbound_messages via its own ad-hoc migration file, which
// would otherwise collide with the 0103_platform_outbound_spine table this
// package's boot path creates in the SAME database.
func TestRealPostgresApply(t *testing.T) {
	dsn := os.Getenv("POSTGRES_PLATFORM_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_PLATFORM_TEST_DSN not set; skipping real PostgreSQL apply validation")
	}
	ctx := context.Background()

	s, err := New(dsn)
	if err != nil {
		t.Fatalf("store.New(postgres dsn): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	applied, err := s.appliedMigrationVersions(ctx)
	if err != nil {
		t.Fatalf("appliedMigrationVersions: %v", err)
	}
	for v := 100; v <= 110; v++ {
		if _, ok := applied[v]; !ok {
			t.Errorf("platform baseline migration %04d missing from schema_migrations", v)
		}
	}

	for _, table := range bootstrapOwnedTables {
		if !s.tableExists(ctx, table) {
			t.Errorf("bootstrap-owned table %q missing after real Postgres boot", table)
		}
	}

	// Double-boot idempotency against the SAME real Postgres database:
	// layer 1 is run-once (schema_migrations), layer 2 must tolerate a
	// second run without error.
	second, err := New(dsn)
	if err != nil {
		t.Fatalf("second boot on same Postgres DSN must be idempotent: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second boot: %v", err)
	}
}
