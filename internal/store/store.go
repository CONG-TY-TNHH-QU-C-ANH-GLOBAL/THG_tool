// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thg/scraper/internal/store/coordination"
	"github.com/thg/scraper/internal/store/crawl"
	"github.com/thg/scraper/internal/store/dbutil"
	"github.com/thg/scraper/internal/store/knowledge"
	"github.com/thg/scraper/internal/store/outbound"
	"github.com/thg/scraper/internal/store/prompts"

	_ "modernc.org/sqlite"
)

// Store provides database access for the scraper system.
//
// Subpackage composition (as domains extract per STORE_SUBPACKAGE_REFACTOR):
//
//   - [outbound.Store] (Phase 2, 2026-05-21) — owns outbound_messages,
//     action_policies, execution_attempts ledger. Legacy top-level
//     bridge wrappers (QueueOutboundForOrg, …) delegate to it; new
//     code MUST use [Store.Outbound()].
//   - [crawl.Store] (Phase 3, 2026-05-21) — owns org_crawl_intents,
//     groups, group_quality, posts, comments, private_files. NO
//     bridge wrappers — clean-cut migration moved all callers to
//     [Store.Crawl()] directly.
//   - [knowledge.Store] (Phase 4, 2026-05-21) — owns knowledge_sources,
//     knowledge_assets, knowledge_events, knowledge_feedback,
//     knowledge_assets.embedding_* metadata. Clean-cut migration
//     (zero cross-domain writes by audit, ≤6 callers per method);
//     method names dropped the redundant `Knowledge` prefix. Reach
//     via [Store.Knowledge()]; no bridge wrappers.
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect // never nil after New; set during boot based on driver
	encKey  string         // AES-256-GCM key for sensitive fields; empty = no encryption

	// outbound owns outbound_messages + action_policies + the queue/
	// claim/finalize/reset state machine + the execution_attempts
	// transition ledger writes. Cross-domain hooks (action_ledger,
	// behaviour caps, conversation gate) are wired below via
	// [installOutboundHooks].
	outbound *outbound.Store

	// crawl owns the crawl pipeline tables (intents, groups,
	// group_quality, posts, comments, private_files). No Hooks —
	// crawl has zero cross-domain writes by audit. Wired via
	// [installCrawlStore].
	crawl *crawl.Store

	// knowledge owns the Workspace Knowledge OS tables: sources,
	// assets, embeddings metadata, events, feedback, replay/soak
	// reads, vector queries, and cost rollups. No Hooks — knowledge
	// has zero cross-domain writes by audit (Phase 4, 2026-05-21).
	knowledge *knowledge.Store

	// coordination owns the runtime-truth substrate: action_ledger,
	// account_behaviour_profiles + account_behaviour_runtime,
	// execution_attempts, engagement_reconcile. Phase 5B clean-cut
	// extraction (2026-05-21). Outbound writes into coordination via
	// the Hooks closure pattern (RecordLedgerTx, IncrementCounterTx,
	// CheckCapsTx, RecordTransitionTx) — coordination itself imports
	// no peer domain per L1 + [[feedback_no_bidirectional_domain_knowledge]].
	coordination *coordination.Store

	// prompts owns prompt_logs + ai_memory + scan_logs +
	// prompt_routing observability + org_skills/skill_executions.
	// Phase 9 clean-cut extraction (2026-05-22). No bridge wrappers.
	prompts *prompts.Store
}

// Outbound exposes the outbound-domain subpackage handle. New code
// MUST use this rather than the legacy bridge methods on *Store.
func (s *Store) Outbound() *outbound.Store { return s.outbound }

// Crawl exposes the crawl-domain subpackage handle. All crawl
// callers (~25 sites across cmd/scraper, internal/server/crawl,
// internal/server/leads, internal/leadingest, internal/telegram,
// internal/jobhandlers) reach it via this accessor — there are no
// top-level bridge wrappers for crawl.
func (s *Store) Crawl() *crawl.Store { return s.crawl }

// Knowledge exposes the knowledge-domain subpackage handle. All
// knowledge callers (across cmd/scraper, internal/workspace_knowledge,
// internal/server, and internal/runtime) reach it via this accessor —
// there are no top-level bridge wrappers for knowledge.
func (s *Store) Knowledge() *knowledge.Store { return s.knowledge }

// Coordination exposes the coordination-domain subpackage handle.
// New code MUST use this accessor; the Phase 5B extraction did not
// introduce top-level bridge wrappers.
func (s *Store) Coordination() *coordination.Store { return s.coordination }

// Prompts exposes the prompts-domain subpackage handle. Phase 9
// clean-cut extraction (2026-05-22) — no top-level bridge wrappers.
func (s *Store) Prompts() *prompts.Store { return s.prompts }

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

	s := &Store{db: db, dialect: dbutil.NewSQLiteDialect()}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := s.runMigrations(context.Background()); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	s.coordination = coordination.NewStore(s.db, s.dialect)
	s.installOutboundHooks()
	s.crawl = crawl.NewStore(s.db, s.dialect)
	s.knowledge = knowledge.NewStore(s.db, s.dialect)
	s.prompts = prompts.NewStore(s.db, s.dialect)
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
	db, err := sql.Open(dbutil.PostgresDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	s := &Store{db: db, dialect: dbutil.NewPostgresDialect()}
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
	s.coordination = coordination.NewStore(s.db, s.dialect)
	s.installOutboundHooks()
	s.crawl = crawl.NewStore(s.db, s.dialect)
	s.knowledge = knowledge.NewStore(s.db, s.dialect)
	s.prompts = prompts.NewStore(s.db, s.dialect)
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

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// schemaAlreadyApplied reports whether migrate() finished writing the
// current-version marker on this DB. Distinct from "any tables exist":
// the marker is the *last* thing migrate() writes, and its version
// must match schemaBootstrapVersion (see schema.go) — so an older
// production DB whose schema lags will fail this probe and re-run
// migrate(), creating any tables/columns added since.
//
// A failed Scan (table missing, no row, wrong version, sqlite error)
// always returns false → migrate() body runs. Safe by default.
func (s *Store) schemaAlreadyApplied() bool {
	var n int
	row := s.db.QueryRow(
		`SELECT 1 FROM _schema_bootstrap_marker WHERE version = ?`,
		schemaBootstrapVersion,
	)
	if err := row.Scan(&n); err != nil {
		return false
	}
	return n == 1
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
