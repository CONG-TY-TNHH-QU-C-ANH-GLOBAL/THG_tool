// Package coordination owns the runtime-truth substrate of the THG sales
// platform: action_ledger, account_behaviour_profiles +
// account_behaviour_runtime, execution_attempts (and verifier outcomes),
// and engagement_reconcile.
//
// Per [DOMAINS.md §2.4](../DOMAINS.md) coordination is the canonical
// owner of these truths. Outbound + leads are PROJECTIONS over the
// coordination-owned tables (per
// [[feedback_no_bidirectional_domain_knowledge]]) — coordination MUST NOT
// import outbound or leads. The cross-package writes that outbound's
// queue/finalize paths perform reach coordination via the
// outbound.Hooks closure pattern wired in
// `installOutboundHooks` (internal/store/outbound_hooks.go).
//
// Phase 5B extraction (2026-05-21): moved from top-level store/ as a
// mechanical clean-cut migration. Pre-existing append-only violations
// (MarkActionLedgerOutcome* UPDATEs, engagement_reconcile UPDATEs)
// carried as documented debt; the fix is a follow-up PR per
// [[feedback_append_only_correction_events]]. Per the user mandate
// 2026-05-21 ("preserve byte-for-byte"), this PR introduces zero
// semantic change.
package coordination

import (
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Store provides coordination-domain data access. Wired into the
// top-level store.Store via composition; reached via
// `store.Store.Coordination()`.
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
}

// NewStore constructs a coordination Store. The top-level store.New is
// the only legitimate production caller; tests reach it via the
// storetest CopyTemplate pattern (see testing_helpers_test.go).
func NewStore(db *sql.DB, dialect dbutil.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// DB returns the underlying *sql.DB. Reserved for test code paths that
// need to plant rows directly (bypass the canonical write path).
// Production callers MUST go through specific methods.
func (s *Store) DB() *sql.DB { return s.db }

// Dialect returns the SQL flavour used by this Store. Coordination has
// no dialect-divergent SQL today; carrying the dialect for future use
// and parity with sibling subpackages.
func (s *Store) Dialect() dbutil.Dialect { return s.dialect }
