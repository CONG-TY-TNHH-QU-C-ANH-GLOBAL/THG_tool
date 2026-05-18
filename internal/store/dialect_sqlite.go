package store

import (
	"context"
	"database/sql"
	"fmt"
)

// sqliteDialect is the SQLite flavor of [Dialect]. SQLite is the
// default development driver and the dialect every test runs against
// today. The implementation is the identity on Rebind (SQLite accepts
// `?` placeholders natively) and uses SQLite-native time functions.
//
// SQLite version requirement: 3.35+ for RETURNING support. modernc.org/sqlite
// embeds a recent SQLite (currently 3.46.x). Verified at boot
// indirectly — any failing RETURNING query surfaces as a clear error
// at the first INSERT, not a silent fallback.
type sqliteDialect struct{}

func (sqliteDialect) Name() string { return "sqlite" }

// Rebind is the identity for SQLite — `?` is the native placeholder.
func (sqliteDialect) Rebind(q string) string { return q }

func (sqliteDialect) NowExpr() string { return "CURRENT_TIMESTAMP" }

func (sqliteDialect) IntervalDaysExpr(days int) string {
	// SQLite modifier syntax: 'now' minus 'N days' formatted to a
	// DATETIME string. Comparisons against TEXT-stored datetimes
	// rely on the YYYY-MM-DD HH:MM:SS lexicographic ordering — true
	// for every column we use (set via CURRENT_TIMESTAMP default).
	return fmt.Sprintf("DATETIME('now', '-%d days')", days)
}

// InsertReturningID for SQLite uses QueryRowContext + Scan against
// the RETURNING clause. We could also use ExecContext + LastInsertId
// on SQLite specifically; using RETURNING keeps the call site dialect-
// agnostic (caller writes one SQL template that works on both).
func (sqliteDialect) InsertReturningID(ctx context.Context, db *sql.DB, query string, args ...any) (int64, error) {
	var id int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}
