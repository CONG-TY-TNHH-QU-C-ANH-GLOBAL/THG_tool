package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store provides database access for the scraper system.
type Store struct {
	db      *sql.DB
	dialect Dialect // never nil after New; set during boot based on driver
	encKey  string  // AES-256-GCM key for sensitive fields; empty = no encryption
}

// New creates a new Store, initializing the database and running
// migrations. dbPath is interpreted as follows:
//
//   - empty → fall back to env DATABASE_URL (PG mode if non-empty,
//     else error)
//   - starts with "postgres://" or "postgresql://" → PG connection
//     string (driver: pgx/v5/stdlib, registered separately)
//   - anything else → treated as a SQLite file path
//
// The dialect is determined at this point and is constant for the
// lifetime of the Store. All subsequent code paths reach the dialect
// via *Store.Dialect() — never re-detect.
//
// Production guidance: see specs/POSTGRES_COMPAT_PLAN.md §4 for the
// rollout sequence and §3.6 for test infrastructure.
func New(dbPath string) (*Store, error) {
	// DATABASE_URL takes precedence when dbPath is empty. This is the
	// production cutover hook — operators set DATABASE_URL in the
	// deployment config and existing dev callers (which pass file
	// paths) are unaffected.
	if dbPath == "" {
		dbPath = strings.TrimSpace(os.Getenv("DATABASE_URL"))
		if dbPath == "" {
			return nil, fmt.Errorf("store.New: empty dbPath and DATABASE_URL not set")
		}
	}

	if isPostgresDSN(dbPath) {
		return newPostgres(dbPath)
	}
	return newSQLite(dbPath)
}

// newSQLite opens the SQLite driver with the pragmas the existing
// codebase relies on. Behaviour preserved verbatim from the previous
// implementation — only the dialect-wiring is new.
//
// Connection pool: we deliberately leave MaxOpenConns at the
// database/sql default (unlimited). Two reasons:
//
//   - Existing code paths (outbound.QueueOutboundForOrg → nested
//     ResolveAccountCaps inside an open Tx) require more than one
//     connection per goroutine. Pinning the pool would deadlock
//     those paths under any non-trivial concurrency.
//   - The CI hang we saw under the race detector was on db.Close(),
//     not on query execution. Bounding the close (see Close() below)
//     surfaces the hang as ErrCloseTimedOut so CI fails diagnosably
//     instead of running out the 120s timeout.
//
// If a future PR refactors the nested-query-inside-Tx pattern to
// use tx-bound helpers, MaxOpenConns can drop to 1 and SQLite's
// engine-level write serialisation becomes the pool boundary.
func newSQLite(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// busy_timeout=15000ms gives concurrent writers ~15s to wait for a
	// held write lock before SQLITE_BUSY surfaces. CI machines under
	// load still flaked at 5s when 8+ goroutines raced
	// QueueOutboundForOrg; combined with retryOnBusy in helpers.go this
	// is the belt-and-braces fix.
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(15000)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Store{db: db, dialect: sqliteDialect{}}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := s.runMigrations(context.Background()); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return s, nil
}

// newPostgres opens the PostgreSQL driver. Pool tuning addresses R12
// in POSTGRES_COMPAT_PLAN.md:
//
//   - MaxOpenConns 25: enough for the agent runtime + 2 workers + UI
//     without saturating a small production PG. Tunable via env later.
//   - MaxIdleConns 5: keep a warm pool but do not hold many idle.
//   - ConnMaxLifetime 5m: refresh connections regularly so cloud-PG
//     proxies (e.g. RDS Proxy) can rebalance.
//
// Driver registration happens via a build tag in postgres_driver.go
// — keeps the standard build from depending on pgx unless PG is wanted.
func newPostgres(dsn string) (*Store, error) {
	db, err := sql.Open(postgresDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	s := &Store{db: db, dialect: postgresDialect{}}
	// On a brand-new PG DB, s.migrate() (the SQLite-style baseline)
	// would fail because its DDL is SQLite-flavour. The PG baseline
	// MUST land via a 0001 migration file. Until that file exists,
	// store.New(postgres://...) errors clearly — operators get a
	// migration-missing message, not a confusing schema error.
	if !s.tableExists(context.Background(), "schema_migrations") {
		// Run the legacy SQLite migrate for SQLite only; PG must rely
		// on migrations/0001_baseline__postgres.up.sql (added in PR
		// that finalizes PG support).
	}
	if err := s.runMigrations(context.Background()); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return s, nil
}

// isPostgresDSN heuristically detects a Postgres connection string.
// Both `postgres://` and `postgresql://` are RFC-recognised forms;
// `host=... port=...` keyword form is the older libpq style.
func isPostgresDSN(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(low, "postgres://"):
		return true
	case strings.HasPrefix(low, "postgresql://"):
		return true
	case strings.Contains(low, "host=") && strings.Contains(low, "user="):
		return true
	}
	return false
}

// closeTimeout is the upper bound Close() will wait for the underlying
// sql.DB to release its handle. Picked to be longer than any sane
// SQLite checkpoint (the only thing modernc.org/sqlite blocks on
// during Close in WAL mode) but short enough to surface a real hang
// inside a CI test timeout rather than swallowing the whole 120s.
const closeTimeout = 10 * time.Second

// ErrCloseTimedOut is returned when Close() did not observe the
// underlying sql.DB shutting down within closeTimeout. It indicates
// a leaked rows/stmt/tx somewhere — Close() blocks until every
// in-flight statement finishes — and is a real test failure, not a
// flake to retry. The Close caller's defer still runs; the timeout
// just unblocks the goroutine.
var ErrCloseTimedOut = fmt.Errorf("store.Close: db did not close within %s (leaked rows/stmt/tx?)", closeTimeout)

// Close closes the database connection with a bounded wait. database/sql.Close
// is documented to "wait for all queries that have started processing on
// the server to finish" — under modernc.org/sqlite + the race detector,
// that can hang indefinitely if a test forgot to Close a *sql.Rows or
// left a transaction open. We translate that hang into ErrCloseTimedOut
// so CI fails loud with a diagnosable error instead of running out the
// `-timeout 120s` budget.
func (s *Store) Close() error {
	done := make(chan error, 1)
	go func() { done <- s.db.Close() }()
	select {
	case err := <-done:
		return err
	case <-time.After(closeTimeout):
		return ErrCloseTimedOut
	}
}

// DB returns the underlying *sql.DB for packages that need direct SQL access
// (e.g. session.StateMachine, session.CheckpointManager).
//
// NOTE: callers reaching for DB() directly bypass the dialect layer.
// That is fine for SQL that is identical on every dialect (most reads).
// SQL that uses `CURRENT_TIMESTAMP`, intervals, or `?` placeholders
// MUST route through s.Dialect() or it WILL break on Postgres.
func (s *Store) DB() *sql.DB { return s.db }

// Dialect returns the SQL flavor the store was opened against.
// Repository code that constructs dialect-divergent SQL must read
// this once per call — see specs/POSTGRES_COMPAT_PLAN.md §3.
func (s *Store) Dialect() Dialect { return s.dialect }

// SetEncryptionKey sets the AES-256-GCM key used to encrypt sensitive DB fields
// (cookies_json, proxy_url). Must be called before any account operations.
func (s *Store) SetEncryptionKey(key string) {
	s.encKey = key
}
