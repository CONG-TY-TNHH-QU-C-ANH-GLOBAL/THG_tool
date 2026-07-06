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

// insertReturningID is the cross-dialect alternative to ExecContext +
// LastInsertId. query MUST already be a static, dialect-correct SQL literal
// — see reels.go/scripts.go's per-statement *Query(dialect) functions —
// ending in `RETURNING <id_col>`. Unlike internal/store/knowledge (which
// runs on both dialects and rebinds a single `?`-templated query at
// runtime), reel is Postgres-only: each statement is written once per
// dialect as a source-literal switch, so no runtime query rewriting
// (Rebind/Sprintf/concatenation) ever touches a SQL string here.
func (s *Store) insertReturningID(ctx context.Context, query string, args ...any) (int64, error) {
	return s.dialect.InsertReturningID(ctx, s.db, query, args...)
}
