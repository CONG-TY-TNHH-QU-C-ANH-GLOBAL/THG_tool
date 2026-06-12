// Domain: coordination (see internal/store/DOMAINS.md)
package coordination

import (
	"context"
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
)

// CapsDecision + DecideCaps (the pure policy) live in decide_caps.go;
// this file keeps the DB-reading wrappers that feed them.

// checkBehaviourCapsTx runs the Coordination Plane PR-2 enforcement
// layer against an open queue transaction. Reasons returned:
//
//   - account_cooldown_active : cooldown_until is in the future
//   - daily_limit_exceeded    : today-counter has reached the cap
//   - risk_ceiling_exceeded   : risk_score >= preset ceiling
//
// Profile-missing is NOT an error — a fresh account inherits the
// TrustWarming preset.
//
// Phase 2 of STORE_SUBPACKAGE_REFACTOR: this function lives at the
// top-level store package and is wired into [outbound.Hooks.BehaviourCheck]
// at construction time (see outbound_aliases.go::installOutboundHooks).
// When coordination is extracted as its own subpackage (Phase 5B), this
// function moves into `internal/store/coordination/` and the hook
// continues to point at it via the same closure indirection. The
// outbound adapter handles the CapsDecision -> GuardDecision conversion.
//
// tenant-ok: cross-domain projection (outbound -> coordination). The
// account_runtime_state table is owned by the coordination domain;
// outbound queries it only via this injected hook.
//
// Phase 5B: exported as CheckCapsTx (was checkBehaviourCapsTx) because
// the hooks closure now lives across the package boundary.
func (s *Store) CheckCapsTx(tx *sql.Tx, accountID int64, msgType string) (CapsDecision, error) {
	caps, _, err := s.ResolveAccountCaps(context.Background(), accountID)
	if err != nil {
		return CapsDecision{}, err
	}

	// Apply time-based decay BEFORE reading risk_score so a previously
	// over-ceiling account that has been idle long enough recovers without
	// needing operator reset. Decay only writes when there is something to
	// decay; missing rows are a no-op.
	if err := ApplyRiskDecayTx(tx, accountID); err != nil {
		return CapsDecision{}, err
	}

	// Single round-trip: read every column the cap decision needs in
	// one SELECT, then apply day-rollover + cap check in Go.
	var (
		countersDay                                                   string
		commentsToday, inboxToday, groupPostsToday, profilePostsToday int
		riskScore                                                     float64
		cooldownUntilStr                                              string
		actorBlocked                                                  int
	)
	err = tx.QueryRow(
		`SELECT counters_day, comments_today, inbox_today, group_posts_today,
		        profile_posts_today, risk_score, COALESCE(cooldown_until,''),
		        COALESCE(actor_blocked,0)
		   FROM account_runtime_state
		  WHERE account_id = ?`, accountID,
	).Scan(&countersDay, &commentsToday, &inboxToday, &groupPostsToday,
		&profilePostsToday, &riskScore, &cooldownUntilStr, &actorBlocked)
	if err != nil && err != sql.ErrNoRows {
		return CapsDecision{}, err
	}

	var cooldownUntil time.Time
	if cooldownUntilStr != "" {
		cooldownUntil = dbutil.ParseSQLiteTime(cooldownUntilStr)
	}
	// tenant-ok: cross-domain read (coordination -> identities). The admin
	// assignment-pause switch lives on accounts; the gate reads it via this
	// single projection query. Strict: a read error blocks (fail-closed) —
	// a silent not-paused default would defeat the safety switch.
	var assignmentPaused int
	if err := tx.QueryRow(
		`SELECT COALESCE(assignment_paused, 0) FROM accounts WHERE id = ?`, accountID,
	).Scan(&assignmentPaused); err != nil && err != sql.ErrNoRows {
		return CapsDecision{}, err
	}
	return DecideCaps(time.Now().UTC(), caps, countersDay, commentsToday, inboxToday, groupPostsToday,
		profilePostsToday, riskScore, cooldownUntil, actorBlocked == 1, assignmentPaused == 1, msgType), nil
}

// EvaluateCaps is the READ-ONLY cap check for the readiness matrix (PR-D). Unlike
// CheckCapsTx it takes NO transaction and applies NO risk decay (a projection must
// not write) — so a risk score that decay would recover may still read as
// over-ceiling, which is conservatively correct for a "is this account ready"
// display. The actual queue gate (CheckCapsTx) remains authoritative.
func (s *Store) EvaluateCaps(ctx context.Context, accountID int64, msgType string) (CapsDecision, error) {
	caps, _, err := s.ResolveAccountCaps(ctx, accountID)
	if err != nil {
		return CapsDecision{}, err
	}
	var (
		countersDay                                                   string
		commentsToday, inboxToday, groupPostsToday, profilePostsToday int
		riskScore                                                     float64
		cooldownUntilStr                                              string
		actorBlocked                                                  int
	)
	err = s.db.QueryRowContext(ctx,
		`SELECT counters_day, comments_today, inbox_today, group_posts_today,
		        profile_posts_today, risk_score, COALESCE(cooldown_until,''),
		        COALESCE(actor_blocked,0)
		   FROM account_runtime_state
		  WHERE account_id = ?`, accountID,
	).Scan(&countersDay, &commentsToday, &inboxToday, &groupPostsToday,
		&profilePostsToday, &riskScore, &cooldownUntilStr, &actorBlocked)
	if err != nil && err != sql.ErrNoRows {
		return CapsDecision{}, err
	}
	var cooldownUntil time.Time
	if cooldownUntilStr != "" {
		cooldownUntil = dbutil.ParseSQLiteTime(cooldownUntilStr)
	}
	// tenant-ok: cross-domain read (coordination -> identities), mirrors
	// CheckCapsTx so gate and matrix can never disagree on the pause state.
	var assignmentPaused int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(assignment_paused, 0) FROM accounts WHERE id = ?`, accountID,
	).Scan(&assignmentPaused); err != nil && err != sql.ErrNoRows {
		return CapsDecision{}, err
	}
	return DecideCaps(time.Now().UTC(), caps, countersDay, commentsToday, inboxToday, groupPostsToday,
		profilePostsToday, riskScore, cooldownUntil, actorBlocked == 1, assignmentPaused == 1, msgType), nil
}
