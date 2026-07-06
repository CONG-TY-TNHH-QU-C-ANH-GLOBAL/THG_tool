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

	"github.com/thg/scraper/internal/store/dbutil"
)

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

// --- Dialect wrappers (mirrors internal/store/knowledge, the reference
// subpackage shape per internal/store/DOMAINS.md §3) — rebind `?`
// placeholders for the active dialect before handing the query to *sql.DB.

func (s *Store) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.dialect.Rebind(query), args...)
}

func (s *Store) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.dialect.Rebind(query), args...)
}

func (s *Store) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.dialect.Rebind(query), args...)
}

// insertReturningID is the cross-dialect alternative to ExecContext +
// LastInsertId. The query MUST end with `RETURNING <id_col>`.
func (s *Store) insertReturningID(ctx context.Context, query string, args ...any) (int64, error) {
	return s.dialect.InsertReturningID(ctx, s.db, s.dialect.Rebind(query), args...)
}
