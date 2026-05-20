// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Dialect is a type alias for [dbutil.Dialect] so existing call sites
// like `*Store.Dialect() store.Dialect` keep working without forcing
// every caller to import dbutil directly. New code may use either
// `store.Dialect` or `dbutil.Dialect` — they are the same type.
//
// Phase 1 of STORE_SUBPACKAGE_REFACTOR moved the interface itself into
// dbutil so domain subpackages can depend on it without dragging the
// whole *Store god-struct. The wrapper methods below remain on *Store
// because they read the dialect off Store state.
type Dialect = dbutil.Dialect

// --- Dialect-aware *Store wrappers ---
//
// These thin wrappers route a query through Dialect.Rebind() before
// handing it to *sql.DB. New code should prefer these helpers over
// raw db.QueryContext / db.ExecContext when:
//
//   - the query contains `?` placeholders (which it almost always does); OR
//   - the query is dialect-portable.
//
// Legacy code in the store package (the 22 files identified in the
// POSTGRES_COMPAT_PLAN inventory) currently uses raw *sql.DB methods.
// Those methods work today only because every deployed instance runs
// SQLite. Converting them to these wrappers is the per-domain work
// teams pick up when their feature needs PG support — see the
// "Domain PG-readiness" status doc.

// QueryContext runs a SELECT and rebinds placeholders for the dialect.
func (s *Store) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.dialect.Rebind(query), args...)
}

// QueryRowContext runs a single-row SELECT and rebinds placeholders.
func (s *Store) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.dialect.Rebind(query), args...)
}

// ExecContext runs an INSERT/UPDATE/DELETE and rebinds placeholders.
//
// IMPORTANT: do NOT use the returned sql.Result for LastInsertId on
// the Postgres path — see risk R1 in POSTGRES_COMPAT_PLAN.md. Use
// [Store.InsertReturningID] when you need the generated key.
func (s *Store) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.dialect.Rebind(query), args...)
}

// InsertReturningID is the cross-dialect alternative to the
// ExecContext + LastInsertId pattern. The query MUST end with
// `RETURNING <id_col>`. Both SQLite (>=3.35) and Postgres support
// this; no version branching needed.
func (s *Store) InsertReturningID(ctx context.Context, query string, args ...any) (int64, error) {
	return s.dialect.InsertReturningID(ctx, s.db, s.dialect.Rebind(query), args...)
}
