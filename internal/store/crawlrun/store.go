// Package crawlrun is the PostgreSQL-platform store for Facebook crawl-run
// queue and lifecycle operations over facebook_crawl_runs (schema migrations
// 0113–0117). It owns the atomic enqueue / claim / heartbeat / recover
// transactions; per-item freshness and account-assignment POLICY live in the
// neutral internal/services/facebook/crawlcampaign package and never here — the
// store persists already-decided outcomes, it does not decide them.
//
// Dormant (PR-M3B): no scheduler, handler, dispatcher, extension, or runtime
// entrypoint invokes these methods yet. PR-M4 wires a consumer and maps service
// domain types to this package's persistence types at the composition root.
//
// Postgres-only, like internal/store/reel: SQLite carries no crawl-campaign
// schema, so every method fails fast on a non-Postgres dialect. This package
// deliberately does NOT import internal/services/facebook (the reverse-
// dependency boundary guard, check_import_boundaries.sh): it owns its own
// request/response types rather than reaching into the service module.
package crawlrun

import (
	"database/sql"
	"errors"

	"github.com/thg/scraper/internal/store/dbutil"
)

// ErrUnsupportedDialect is returned by every public method when the store was
// constructed against a non-Postgres dialect. There is no SQLite schema for the
// crawl-campaign tables, so this is a configuration error, not a "not found".
var ErrUnsupportedDialect = errors.New("crawlrun: postgres-only store; no schema exists for this dialect")

// ErrInvalidFence signals a caller-side input error: a Fence with a
// non-positive OrgID, RunID, or Attempt. It is distinct from a valid fence that
// simply matches no running row (a stale-worker state) so an adapter/programming
// bug surfaces instead of hiding behind a stale result.
var ErrInvalidFence = errors.New("crawlrun: invalid fence")

// Store is the crawl-run store handle. Construct via [NewStore].
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
}

// NewStore constructs a crawl-run store. Idempotent and cheap — no I/O, no
// migrations. Migrations are owned by the top-level Store and run before any
// method here is called.
func NewStore(db *sql.DB, dialect dbutil.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// requirePostgres is called first by every public method. Every SQL statement
// in this package is a Postgres-only literal ($N placeholders, RETURNING,
// FOR UPDATE SKIP LOCKED), so a non-Postgres dialect never reaches a query.
func (s *Store) requirePostgres() error {
	if s.dialect.Name() != "postgres" {
		return ErrUnsupportedDialect
	}
	return nil
}
