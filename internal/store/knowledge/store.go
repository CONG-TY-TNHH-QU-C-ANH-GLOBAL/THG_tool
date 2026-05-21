// Package knowledge owns the Workspace Knowledge OS tables: sources
// (Layer 1), assets (Layer 3), embeddings (Layer 2.5), events (Layer 7
// observability), feedback (G10 append-only), replay (operator
// timeline), soak (quality metrics), vector queries (Layer 4 pgvector
// retrieval), and cost accounting.
//
// Phase 4 of STORE_SUBPACKAGE_REFACTOR (2026-05-21) extracted this
// domain from the god-package `internal/store`. Audit findings:
//
//   - ZERO cross-domain writes — every INSERT/UPDATE/DELETE targets a
//     knowledge-owned table. No Hooks struct needed.
//   - All public methods have ≤6 callers. Clean-cut migration applied
//     across the codebase; no bridge wrappers (L2).
//   - No cross-domain JOINs. The single read-before-write in
//     UpsertAsset validates source ownership (intra-domain).
//   - Method names: dropped redundant `Knowledge` prefix when leading
//     (GetKnowledgeAsset → GetAsset). Entity-qualified names retained
//     where the package has multiple entities (GetAsset, UpsertSource,
//     RecordSync — disambiguated by entity, not by domain prefix).
//
// Locked invariants enforced:
//
//   - **L1 Dependency direction**: imports `internal/models` (workspace_knowledge
//     submodules for Asset / Source types) and `internal/store/dbutil` only.
//     Top-level store imports knowledge.
//   - **L3 Tx threading**: knowledge methods open their own transactions
//     for the cascade-delete path (DeleteSourceForOrg). No external tx
//     parameter — Phase 4 audit found no cross-package writers.
//   - **L4 No abstraction theater**: concrete *sql.DB + dialect. No
//     repository interfaces, no DI container, no plugin/event-bus.
//
// Tenant isolation: every public SQL query in this package filters by
// `org_id = ?` per the V2 D1 directive — verified by
// scripts/check_tenant_isolation.sh.
package knowledge

import (
	"context"
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Store is the knowledge-domain handle. Construct via [NewStore].
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
}

// NewStore constructs a knowledge store. Idempotent and cheap — no I/O,
// no migrations. Migrations are owned by the top-level Store and run
// before any knowledge method is called.
func NewStore(db *sql.DB, dialect dbutil.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// DB returns the underlying *sql.DB. Exported for tests and for the
// pgvector capability probe (vector_query.go) that runs dialect-
// specific introspection SQL.
func (s *Store) DB() *sql.DB { return s.db }

// --- Dialect wrappers (1:1 with top-level *store.Store) ---
//
// These thin wrappers rebind `?` placeholders for the active dialect
// before handing the query to *sql.DB. Knowledge methods route every
// query through these so the package is dialect-portable when the PG
// migration lands.

func (s *Store) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.dialect.Rebind(query), args...)
}

func (s *Store) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.dialect.Rebind(query), args...)
}

func (s *Store) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.dialect.Rebind(query), args...)
}

// insertReturningID is the cross-dialect alternative to ExecContext +
// LastInsertId. The query MUST end with `RETURNING <id_col>`.
func (s *Store) insertReturningID(ctx context.Context, query string, args ...any) (int64, error) {
	return s.dialect.InsertReturningID(ctx, s.db, s.dialect.Rebind(query), args...)
}
