// Package crawl owns the crawl-pipeline tables: org_crawl_intents
// (recurring crawl plans), groups (crawl target catalog),
// group_quality (relevance scoring), posts + comments (crawled
// content), and private_files (org-scoped uploaded knowledge).
//
// Phase 3 of STORE_SUBPACKAGE_REFACTOR (2026-05-21) extracted this
// domain out of the god-package `internal/store`. Compared to the
// outbound extraction (Phase 2), crawl has:
//
//   - ZERO cross-domain writes — every INSERT/UPDATE/DELETE targets
//     a crawl-owned table. No Hooks struct is needed; the constructor
//     takes only (db, dialect).
//   - Caller blast radius small enough (~25 callsites) for clean-cut
//     migration. No bridge wrappers in the top-level store package;
//     callers reach crawl methods via `store.Store.Crawl()` directly.
//   - One cohesive entity per file (intents, groups, posts, etc.) —
//     no internal interface scaffolding, no plugin registry, no
//     scheduler abstraction. Per [feedback_freeze_abstraction] L4.
//
// Locked invariants enforced here:
//
//   - **L1 Dependency direction**: imports `internal/models` and
//     `internal/store/dbutil` only. Top-level store imports crawl,
//     never the other way around.
//   - **L3 Tx ownership**: crawl methods open and own their own
//     transactions for the scheduler-claim and cursor-advance paths.
//     They never accept a parent `*sql.Tx` because no cross-package
//     write path threads through crawl today.
//   - **L4 No abstraction theater**: concrete `*sql.DB` + dialect.
//     No repository interfaces, no DI containers.
//
// Tenant isolation: every public SQL query in this package filters
// by `org_id = ?` per the V2 D1 directive. The only intra-domain
// JOINs are group_quality ↔ groups (both crawl-owned), which do not
// require `// tenant-ok` annotations because no domain boundary is
// crossed.
package crawl

import (
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Store is the crawl-domain handle. Construct via [NewStore].
//
// Unlike [outbound.Store], crawl has no Hooks field — the domain is
// self-contained at the SQL layer (zero cross-domain writes found in
// Phase 3 audit). If a future feature introduces a cross-domain
// write (e.g., crawl emits an audit-ledger row on intent claim),
// add a Hooks struct then per the same closure pattern outbound uses.
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
}

// NewStore constructs a crawl store. Idempotent and cheap — no I/O,
// no migrations. Migrations are owned by the top-level Store and run
// before any crawl method is called.
func NewStore(db *sql.DB, dialect dbutil.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}
