// Package outbound owns the outbound_messages table and the queue →
// claim → finalize → reset state machine that drives every AI / agent
// / Telegram action that touches Facebook.
//
// Phase 2 of STORE_SUBPACKAGE_REFACTOR (2026-05-21) extracted this
// domain out of the god-package `internal/store` per the V2.5
// precedent that domain ownership lives in subpackages. Pure
// outbound_messages reads/writes live here; cross-domain side effects
// (action_ledger inserts, behaviour_profile counter bumps,
// conversation thread checks) flow through the [Hooks] callbacks the
// top-level store wires at construction time — see [feedback_store_subpackage_locks]
// L1 (no cycles) and L4 (no abstraction theater).
//
// Locked invariants enforced here:
//
//   - **L1 Dependency direction**: this package imports
//     `internal/models` and `internal/store/dbutil` only. It does NOT
//     import `internal/store` — the top-level package depends on us,
//     not the other way around.
//   - **L3 Tx threading**: every state-machine method that participates
//     in a write opens its OWN tx at the [Store.db] level for now (the
//     queue path is self-contained). When future cross-package writes
//     need to share a tx, methods accept an external `*sql.Tx`.
//   - **L4 No abstraction theater**: cross-domain side effects are
//     plain `func(...)` closures stored on [Hooks]. No repository
//     interfaces, no DI containers, no mock factories. The top-level
//     store passes concrete closures referencing its own methods.
//
// Tenant isolation: every SQL query in this package filters by
// `org_id = ?` per the V2 D1 directive — see scripts/check_tenant_isolation.sh.
package outbound

import (
	"context"
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Store is the outbound-domain handle. Construct via [NewStore].
//
// The Store holds its own *sql.DB reference (shared with the parent
// store.Store but addressable independently) plus a dialect for
// placeholder rewriting and a Hooks struct for cross-domain
// side-effects. None of these are mutated post-construction.
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
	hooks   Hooks
}

// Hooks is the cross-domain callback surface — concrete function
// closures, not an interface (per L4). The top-level store.Store
// wires these at construction time, pointing each closure at the
// corresponding method or free function it owns in the legacy
// god-package. As coordination, identities, and threads get extracted
// into their own subpackages (Phase 5+), each closure will be
// re-pointed at the new owner's public API.
//
// All hooks are best-effort from the outbound side: outbound writes
// to outbound_messages and execution_attempts as the load-bearing
// path; if a hook errors, outbound logs and continues. The hook
// owner is responsible for its own correctness invariants.
type Hooks struct {
	// BehaviourCheck runs the account-level cooldown + daily cap + risk
	// ceiling gate. Returns Allowed=false on cooldown_active, daily
	// limit exceeded, or risk ceiling reached. Implemented by
	// coordination domain (account_runtime_state) — outbound calls it
	// inside the queue tx so the SELECT shares a consistent snapshot
	// with the dedup check.
	//
	// tenant-ok: cross-domain projection (outbound -> coordination).
	BehaviourCheck func(tx *sql.Tx, accountID int64, msgType string) (GuardDecision, error)

	// ConversationGate runs the inbox-style thread state check. Called
	// only when an action_policies row has ConversationAware=1.
	// Implemented by threads domain (conversation_threads).
	//
	// tenant-ok: cross-domain projection (outbound -> threads).
	ConversationGate func(ctx context.Context, orgID int64, targetURL, profileURL string, cooldown time.Duration) (GuardDecision, error)

	// RecordActionLedger appends one row to action_ledger when an
	// outbound is successfully queued. Implemented by coordination
	// domain. Best-effort — failure does NOT roll back the outbound
	// INSERT (the ledger is additive coordination data, not the
	// source of truth).
	//
	// tenant-ok: cross-domain projection (outbound -> coordination).
	RecordActionLedger func(tx *sql.Tx, orgID, accountID int64, msgType, targetURL string, outboundID int64, cooldown time.Duration)

	// IncrementCounter bumps the per-account *_today counter for the
	// action type in account_runtime_state. Coordination domain.
	// Best-effort: counter loss is tolerable (queue success is source
	// of truth, counters are policy data).
	//
	// tenant-ok: cross-domain projection (outbound -> coordination).
	IncrementCounter func(tx *sql.Tx, orgID, accountID int64, msgType string)
}

// NewStore constructs an outbound store with the given DB, dialect,
// and cross-domain hooks. The caller (top-level store.Store) is
// responsible for passing well-formed hooks; outbound does not nil-
// check them on each call because that would clutter every state-
// machine method.
//
// Construction is idempotent and cheap — no I/O, no migrations.
// Migrations are owned by the top-level Store and run before any
// outbound method is called.
func NewStore(db *sql.DB, dialect dbutil.Dialect, hooks Hooks) *Store {
	return &Store{db: db, dialect: dialect, hooks: hooks}
}
