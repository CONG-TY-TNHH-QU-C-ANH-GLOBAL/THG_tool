package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// Dialect captures the SQL flavor of the underlying database driver.
// It exists for ONE reason: SQLite + PostgreSQL have small but
// production-critical differences (placeholder syntax, current-time
// expression, interval math, INSERT-with-id ergonomics) that the
// rest of the codebase should not have to think about.
//
// Design rules:
//
//  1. Keep this interface SMALL. Every method added here is dialect
//     drift the team has to maintain on two implementations forever.
//     If a piece of SQL is identical on both dialects, do NOT route
//     it through a dialect method — write it inline.
//
//  2. NEVER take user input as a SQL fragment. The helpers below
//     accept structural arguments (column names, table names) only,
//     all of which are compile-time constants in our codebase. They
//     do NOT take WHERE clauses, ORDER BY, or anything composed from
//     request data. SQL injection surface is therefore exactly zero.
//
//  3. The dialect knows its name and that's it — no version
//     detection, no feature flags. If a feature is dialect-specific
//     (e.g. JSONB queries), it goes in a separate optional method
//     with a clear "ok" return so callers can branch deterministically.
//
// See specs/POSTGRES_COMPAT_PLAN.md for the production-risk inventory
// this interface addresses (R1, R2, R7, R8, R9).
type Dialect interface {
	// Name is the short identifier — "sqlite" or "postgres". Used in
	// boot logs and test selection, never inside SQL.
	Name() string

	// Rebind translates `?` placeholders in a query to the
	// dialect-native form. SQLite passes through unchanged; Postgres
	// rewrites to `$1, $2, ...` in order.
	//
	// CRITICAL CONTRACT: a literal `?` inside a quoted string in the
	// SQL will be naively rewritten. Our codebase never embeds `?`
	// in string literals; if a future contributor needs to, route
	// around this method.
	Rebind(query string) string

	// NowExpr is the SQL expression that evaluates to "current
	// timestamp" — addresses R7 (CURRENT_TIMESTAMP vs NOW()).
	// SQLite: "CURRENT_TIMESTAMP"
	// PG:     "NOW()"
	NowExpr() string

	// IntervalDaysExpr returns the SQL expression for "now minus N
	// days" — addresses R8 (DATETIME('now', '-Nd') doesn't exist on
	// PG). `days` is inlined as a literal integer; no placeholder
	// because the SQL planner needs the value for index selection.
	//
	// SQLite: "DATETIME('now', '-30 days')"
	// PG:     "NOW() - INTERVAL '30 days'"
	IntervalDaysExpr(days int) string

	// InsertReturningID runs an INSERT and returns the generated
	// primary key. Replaces the LastInsertId() pattern which does
	// not work on the standard PG driver — addresses R1.
	//
	// The query MUST end in `RETURNING <id_col>` for the dialect
	// to surface the new row's ID. Both SQLite (>=3.35) and PG
	// support this syntax. The helper exists so callers do not have
	// to remember which driver supports which return mechanism.
	InsertReturningID(ctx context.Context, db *sql.DB, query string, args ...any) (int64, error)
}

// rebindNumbered rewrites a `?`-placeholder query to `$N` form for
// drivers that require positional dollar-prefixed placeholders
// (the standard PostgreSQL convention). Shared by the PG dialect
// and reusable for any future driver with the same convention.
//
// Implementation note: a single pass, no string-escape awareness —
// see Dialect.Rebind contract about literal `?` in string literals.
func rebindNumbered(query string) string {
	if !strings.ContainsRune(query, '?') {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8) // rough headroom for `$NN` expansion
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// errDialectUnset is returned by store methods that need a dialect
// but were constructed before one was wired. Should never escape
// to production — every store.New code path sets a dialect.
var errDialectUnset = fmt.Errorf("store: dialect not set (programmer error)")

// --- Dialect-aware wrappers on *Store ---
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
