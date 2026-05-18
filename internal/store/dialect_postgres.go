package store

import (
	"context"
	"database/sql"
	"fmt"
)

// postgresDialect is the PostgreSQL flavor of [Dialect]. It is
// production-target for the Knowledge OS once the schema layer is
// ready and the operator has provisioned a PG instance.
//
// Driver compatibility: this dialect is intentionally driver-agnostic
// — it works with both `pgx/v5/stdlib` and `lib/pq`. We register pgx
// in store.go as the default; either driver accepts `$N` placeholders.
//
// Time semantics: PG `NOW()` returns TIMESTAMPTZ in UTC when the
// session timezone is UTC (the recommended setting; we do not change
// the session locally). All timestamp columns in the PG schema are
// TIMESTAMPTZ, so round-trips through Go are timezone-safe.
type postgresDialect struct{}

func (postgresDialect) Name() string { return "postgres" }

// Rebind rewrites `?` to `$N`. See [rebindNumbered] for the contract.
func (postgresDialect) Rebind(q string) string {
	return rebindNumbered(q)
}

func (postgresDialect) NowExpr() string { return "NOW()" }

func (postgresDialect) IntervalDaysExpr(days int) string {
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
// exists. See risk R1 in POSTGRES_COMPAT_PLAN.md.
func (postgresDialect) InsertReturningID(ctx context.Context, db *sql.DB, query string, args ...any) (int64, error) {
	var id int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}
