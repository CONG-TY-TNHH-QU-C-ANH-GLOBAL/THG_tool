// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"fmt"
)

// dbExecer is the surface runMigrationsOn needs — satisfied by both *sql.DB
// (SQLite, and Postgres when no lock is held) and *sql.Conn (the single
// locked Postgres session in runMigrations below). *sql.Tx does NOT satisfy
// this (no BeginTx), which is why recordMigrationVersion in migrator.go
// keeps taking the narrower [execer] instead.
type dbExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// migrationAdvisoryLockKey is an arbitrary, fixed session-advisory-lock key
// (Postgres only — see runMigrations). Picked once and never reused for any
// other lock in this codebase; the exact value has no meaning beyond being
// a stable constant both racing instances agree on.
const migrationAdvisoryLockKey int64 = 875_302_114_001

// runMigrations is the top-level entry point called by store.New AFTER the
// legacy [Store.migrate] has bootstrapped (or, on a brand-new PG database,
// has done nothing). The actual apply logic (ensure schema_migrations,
// record baseline, apply pending migrations) lives in runMigrationsOn
// (migrator.go) — this function's only job is choosing what connection that
// logic runs on.
//
// SQLite is single-writer (file-locked) and single-process here, so it runs
// straight over the pool (s.db) exactly as before this lock was added — no
// behavior change on that path.
//
// Postgres: more than one app instance can boot against the SAME database
// concurrently, so the whole body below runs under a session-level
// pg_advisory_lock — only one instance applies migrations at a time; the
// rest block in Postgres (not spin/retry) until the lock holder finishes,
// then each proceeds no-op (every migration it would apply is already
// recorded in schema_migrations). This replaces the prior race (two
// instances could both pass the "not yet applied" check and one would then
// fail the unique version insert).
//
// The lock and the ENTIRE migration apply happen on ONE dedicated *sql.Conn
// (never the pool, never s.db) — session-level advisory locks are tied to
// the Postgres backend process, not to the Go-level connection wrapper, so
// running any step of the apply on a different pooled connection would not
// actually be covered by the lock. pg_advisory_xact_lock was not used
// instead: the apply spans MULTIPLE transactions (one per migration via
// applyMigration), and a xact-scoped lock releases at the first commit —
// too early to serialize the rest of the chain.
func (s *Store) runMigrations(ctx context.Context) error {
	if s.dialect.Name() != "postgres" {
		return s.runMigrationsOn(ctx, s.db)
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire postgres migration connection: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	// Always unlock on the SAME conn, even if ctx is cancelled/timed out by
	// the time we get here — an un-released session lock would wedge every
	// future boot against this database until the process exits.
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockKey)
	}()

	return s.runMigrationsOn(ctx, conn)
}
