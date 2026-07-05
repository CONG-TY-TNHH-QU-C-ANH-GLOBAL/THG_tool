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

	"github.com/thg/scraper/internal/runtime/events"
	"github.com/thg/scraper/internal/store/app"
	"github.com/thg/scraper/internal/store/connectors"
	"github.com/thg/scraper/internal/store/coordination"
	"github.com/thg/scraper/internal/store/crawl"
	"github.com/thg/scraper/internal/store/dbutil"
	"github.com/thg/scraper/internal/store/identities"
	"github.com/thg/scraper/internal/store/knowledge"
	"github.com/thg/scraper/internal/store/leads"
	"github.com/thg/scraper/internal/store/outbound"
	"github.com/thg/scraper/internal/store/prompts"
	"github.com/thg/scraper/internal/store/sessions"
	"github.com/thg/scraper/internal/store/telegram"
	"github.com/thg/scraper/internal/store/threads"

	// Pure-Go SQLite driver; registers itself with database/sql via its init().
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

	// connectors owns the Chrome-extension bridge tables: agent_tokens
	// (reclassified from identities to connectors in Phase 7),
	// connector_commands, connector_screenshots, connector_pairing,
	// selector_cache. Phase 7 clean-cut extraction (2026-05-22).
	connectors *connectors.Store

	// identities owns the accounts table (FB account identity records
	// with encrypted cookies) + facebook_status summary. Phase 6
	// clean-cut extraction (2026-05-22). encKey is mirrored from
	// top-level Store at construction + via SetEncryptionKey.
	identities *identities.Store

	// app owns the heterogeneous application tables (career_jobs, kpi,
	// media_assets, price_items, stats) plus, since the AppStore
	// dissolution (PR6, 2026-07-05), the app_tasks/task_leads cluster
	// and its app.Migrate bootstrap. The legacy *AppStore wrapper is gone.
	app *app.Store

	// threads owns conversation_threads + conversation_messages.
	// Phase 8a clean-cut extraction (2026-05-22).
	threads *threads.Store

	// leads owns the lead pipeline: leads + lead_engagement projection
	// + classification_log + context_niches. Phase 8b clean-cut
	// extraction (2026-05-22). Holds a *threads.Store reference for
	// the engagement-projection cross-domain reads (per DOMAINS.md §2.2
	// cross-domain projections via // tenant-ok annotations).
	leads *leads.Store

	// telegram owns the Telegram integration control-plane tables
	// (settings, bind_codes, bindings, alert_prefs, audit). Org-scoped,
	// channel-neutral, zero cross-domain writes. See
	// specs/OMNICHANNEL_SALES_COPILOT_TELEGRAM_TRACK.md.
	telegram *telegram.Store

	// sessions owns the browser_sessions table (Chrome/Docker browser
	// lifecycle + LocalSessionStatus enum). PR1 of the *AppStore
	// dissolution (2026-07-01) — mechanical move from the legacy
	// *AppStore-receiver sessions.go/session_status.go. Top-level type
	// aliases + *AppStore bridge methods keep existing callers compiling
	// unchanged; see internal/store/sessions.go for the bridge.
	sessions *sessions.Store
}

// Telegram exposes the telegram-domain subpackage handle (integration
// control-plane). Reach it via this accessor; no top-level bridge wrappers.
func (s *Store) Telegram() *telegram.Store { return s.telegram }

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

// Connectors exposes the connectors-domain subpackage handle. Phase 7
// clean-cut extraction (2026-05-22) — no top-level bridge wrappers.
func (s *Store) Connectors() *connectors.Store { return s.connectors }

// Identities exposes the identities-domain subpackage handle. Phase 6
// clean-cut extraction (2026-05-22) — no top-level bridge wrappers.
func (s *Store) Identities() *identities.Store { return s.identities }

// App exposes the app-domain subpackage handle. Phase 11 narrow scope
// (2026-05-22) — no top-level bridge wrappers.
func (s *Store) App() *app.Store { return s.app }

// Threads exposes the threads-domain subpackage handle. Phase 8a
// clean-cut extraction (2026-05-22).
func (s *Store) Threads() *threads.Store { return s.threads }

// Leads exposes the leads-domain subpackage handle. Phase 8b clean-cut
// extraction (2026-05-22).
func (s *Store) Leads() *leads.Store { return s.leads }

// Sessions exposes the sessions-domain subpackage handle. PR1 of the
// *AppStore dissolution (2026-07-01) — new code should call this directly;
// existing *AppStore callers keep working via the bridge in
// internal/store/sessions.go.
func (s *Store) Sessions() *sessions.Store { return s.sessions }

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
	// Schema is built entirely by the migration runner: a fresh DB gets the
	// 0001_legacy_baseline__sqlite migration (the canonical SQLite schema) plus
	// any later NNNN migrations. No more in-code migrate() bootstrap — the SQL
	// lives in internal/store/migrations/ as the single source of truth, applied
	// transactionally + fail-fast (see migrator.go).
	if err := s.runMigrations(context.Background()); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	if err := s.initDomains(); err != nil {
		return nil, err
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
	// Schema for BOTH dialects is owned entirely by the migration runner
	// (internal/store/migrations/*.up.sql) — there is no in-code baseline
	// anymore. The SQLite baseline lives in 0001_legacy_baseline__sqlite.up.sql;
	// the Postgres baseline (0001_legacy_baseline__postgres.up.sql) lands with
	// the POSTGRES_COMPAT effort. On a brand-new PG DB with no PG baseline file
	// yet, runMigrations applies nothing and the first table access fails
	// clearly — operators get a migration-missing signal, not a silent half-boot.
	if err := s.runMigrations(context.Background()); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	if err := s.initDomains(); err != nil {
		return nil, err
	}
	return s, nil
}

// initDomains wires every domain subpackage and runs the local-runtime
// bootstrap layer in ONE deterministic order. Called by both dialect
// constructors AFTER runMigrations: the versioned migrations own the
// platform-plane schema; sessions.Migrate + app.Migrate own only the
// local-runtime-plane tables (browser_sessions, app_tasks/task_leads,
// browser infra) — see internal/store/migrations/README.md "Bootstrap
// layers". Extracted verbatim from the previously duplicated
// newSQLite/newPostgres blocks (2026-07-05): same calls, same order,
// same error wrapping — the single source of bootstrap determinism.
func (s *Store) initDomains() error {
	s.coordination = coordination.NewStore(s.db, s.dialect)
	s.installOutboundHooks()
	s.crawl = crawl.NewStore(s.db, s.dialect)
	s.knowledge = knowledge.NewStore(s.db, s.dialect)
	s.prompts = prompts.NewStore(s.db, s.dialect)
	s.connectors = connectors.NewStore(s.db, s.dialect)
	s.identities = identities.NewStore(s.db, s.dialect, s.encKey)
	s.app = app.NewStore(s.db, s.dialect)
	s.threads = threads.NewStore(s.db, s.dialect)
	s.leads = leads.NewStore(s.db, s.dialect, s.threads)
	s.telegram = telegram.NewStore(s.db, s.dialect, s.encKey)
	if err := sessions.Migrate(s.db); err != nil {
		return fmt.Errorf("sessions migrate: %w", err)
	}
	s.sessions = sessions.NewStore(s.db, s.dialect)
	if err := app.Migrate(s.db); err != nil {
		return fmt.Errorf("app migrate: %w", err)
	}
	s.installRuntimeEventSink()
	return nil
}

// installRuntimeEventSink wires the events.Sink hook to persist every
// typed runtime event into runtime_events via coordination. Called
// once at the end of New (after all subpackages exist). Per
// [[feedback_freeze_abstraction]] this is a function-typed
// registration, not an interface — the events package never knows
// about the Store type.
func (s *Store) installRuntimeEventSink() {
	events.SetSink(func(ctx context.Context, level, eventName string, attrs []any) {
		_ = s.coordination.RecordRuntimeEvent(ctx, level, eventName, attrs)
	})
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
// Propagates to the identities subpackage so its accounts methods can
// encrypt without reaching back into the parent.
func (s *Store) SetEncryptionKey(key string) {
	s.encKey = key
	if s.identities != nil {
		s.identities.SetEncryptionKey(key)
	}
	if s.telegram != nil {
		s.telegram.SetEncryptionKey(key)
	}
}
