// Package reel owns the Reel Studio foundation tables: reels (task
// metadata) and reel_scripts (versioned script drafts). PostgreSQL-only
// platform-plane schema (docs/architecture/decisions/ADR-reel-studio-platform-module.md)
// — SQLite carries no reel schema, so this package's methods only work
// against a Postgres-backed *Store.
//
// PR-R1 (2026-07-06): schema + store foundation only. No service, no HTTP,
// no provider/render code — those land in later PRs per the ADR's PR train.
// Zero cross-domain writes; no Hooks struct is needed.
package reel

import (
	"context"
	"database/sql"
	"errors"

	"github.com/thg/scraper/internal/store/dbutil"
)

// ErrUnsupportedDialect is returned by every public Store method when the
// store was constructed against a non-Postgres dialect. There is no SQLite
// (or other) schema for the reel tables — see the package doc — so this is
// a configuration error, not a "not found" result; callers must not confuse
// it with sql.ErrNoRows.
var ErrUnsupportedDialect = errors.New("reel: postgres-only store; no schema exists for this dialect")

// Store is the reel-domain handle. Construct via [NewStore].
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
}

// NewStore constructs a reel store. Idempotent and cheap — no I/O, no
// migrations. Migrations are owned by the top-level Store and run before
// any reel method is called.
func NewStore(db *sql.DB, dialect dbutil.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// requirePostgres is called first by every public method, before any query
// runs. Every reel SQL statement is a Postgres-only const literal (see
// reels.go/scripts.go) — this guard is what makes that safe: a non-Postgres
// dialect never reaches a query at all, it fails here with a clear error.
func (s *Store) requirePostgres() error {
	if s.dialect.Name() != "postgres" {
		return ErrUnsupportedDialect
	}
	return nil
}

// insertReturningID runs an INSERT ... RETURNING <id_col> and returns the
// new row's id. query MUST be one of the const SQL literals declared in
// reels.go/scripts.go — never a computed or formatted string.
func (s *Store) insertReturningID(ctx context.Context, query string, args ...any) (int64, error) {
	return s.dialect.InsertReturningID(ctx, s.db, query, args...)
}
