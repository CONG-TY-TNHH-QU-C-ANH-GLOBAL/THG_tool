package dbutil

import (
	"context"
	"database/sql"
	"fmt"
)

// PostgresDialect is the PostgreSQL flavor of [Dialect]. Production-
// target for the Knowledge OS once the schema layer is ready and the
// operator has provisioned a PG instance.
//
// Driver compatibility: this dialect is intentionally driver-agnostic
// — it works with both `pgx/v5/stdlib` and `lib/pq`. We register pgx
// in store.go as the default; either driver accepts `$N` placeholders.
//
// Time semantics: PG `NOW()` returns TIMESTAMPTZ in UTC when the
// session timezone is UTC (the recommended setting; we do not change
// the session locally). All timestamp columns in the PG schema are
// TIMESTAMPTZ, so round-trips through Go are timezone-safe.
type PostgresDialect struct{}

// NewPostgresDialect returns the singleton Postgres dialect.
func NewPostgresDialect() Dialect { return PostgresDialect{} }

func (PostgresDialect) Name() string { return "postgres" }

// Rebind rewrites `?` to `$N`. See [RebindNumbered] for the contract.
func (PostgresDialect) Rebind(q string) string {
	return RebindNumbered(q)
}

func (PostgresDialect) NowExpr() string { return "NOW()" }

func (PostgresDialect) IntervalDaysExpr(days int) string {
	// PG accepts integer-valued intervals inline. Safe because `days`
	// is a Go int from a constant call site, never user input. We
	// stringify with %d to keep the planner-visible literal stable
	// across query parses (PG caches plans by exact SQL text).
	return fmt.Sprintf("NOW() - INTERVAL '%d days'", days)
}

// InsertReturningID expects the caller to terminate the query with
// `RETURNING <id_col>`. PG returns the value via QueryRow + Scan; the
// stdlib lib/pq driver does NOT support `LastInsertId()` at all
// (returns ErrNoLastInsertID), which is the entire reason this method
// exists. See risk R1 in specs/domains/knowledge-platform/features/knowledge-os/implementation/postgres-compat.md.
func (PostgresDialect) InsertReturningID(ctx context.Context, db *sql.DB, query string, args ...any) (int64, error) {
	var id int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// PostgresDriverName is the database/sql driver name the store uses
// when DATABASE_URL points to Postgres. The driver itself is NOT
// imported here — keeping the dependency optional means the SQLite-
// only builds (dev, tests, hot-fix branches) do not have to pull pgx
// into their dependency graph.
//
// To enable PG support in a binary, add a blank import to your main
// package:
//
//	// In cmd/scraper/main.go (or wherever your binary's main lives):
//	import _ "github.com/jackc/pgx/v5/stdlib"
//
// The pgx stdlib package registers itself under the "pgx" driver name
// on init. If you forget the import, store.New() with a PG DSN fails
// at sql.Open with "unknown driver pgx" — clear enough.
const PostgresDriverName = "pgx"
