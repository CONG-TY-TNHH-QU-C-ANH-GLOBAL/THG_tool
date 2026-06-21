package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open creates a pgx connection pool for the given DSN.
//
// It is used by the gated integration tests and by future explicit operator
// tooling during the PostgreSQL cutover. It is deliberately NOT called by
// application startup — current runtime DB initialization (SQLite, via
// internal/store) is unchanged. Callers own the returned pool's lifecycle and
// must Close it.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, dsn)
}
