// Domain: coordination (see internal/store/DOMAINS.md)
package coordination

import (
	"context"
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/dbutil"
)

// CapsDecision is the coordination-domain primitive returned by
// [Store.checkBehaviourCapsTx]. It carries the cap-check result in a
// shape that does NOT reference any peer domain — the outbound layer's
// hook adapter (outbound_aliases.go::installOutboundHooks) converts
// this into [outbound.GuardDecision] at its boundary.
//
// Why a coordination-local type: coordination is BELOW outbound in the
// dependency graph (DOMAINS.md §2). When the coordination subpackage
// is extracted (Phase 5B), it cannot import outbound — that would be
// the bidirectional-knowledge violation locked in
// [[feedback_no_bidirectional_domain_knowledge]]. Decouple-1
// (2026-05-21) introduced this primitive as pre-work so the 5B move is
// mechanical.
//
// Field semantics:
//
//   - Allowed: whether the action passes the cap check.
//   - Reason: short tag — "ok" | "account_cooldown_active" |
//     "daily_limit_exceeded" | "risk_ceiling_exceeded". Stable strings
//     consumed by the outbound dedup/queue layer for telemetry.
//   - CooldownUntil: zero unless Reason == "account_cooldown_active";
//     when set, carries the wall-clock instant after which the cap
//     would re-allow the action. Outbound's adapter maps this into
//     GuardDecision.LastOutboundAt for back-compat with the existing
//     consumer shape.
type CapsDecision struct {
	Allowed       bool
	Reason        string
	CooldownUntil time.Time
	// RiskScore and RiskCeiling are populated ONLY when
	// Reason == "risk_ceiling_exceeded". They expose the gate's inputs so
	// the operator-facing telemetry can show "why was this blocked?"
	// without a separate diagnostic round-trip.
	RiskScore   float64
	RiskCeiling float64
}

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
	return DecideCaps(time.Now().UTC(), caps, countersDay, commentsToday, inboxToday, groupPostsToday,
		profilePostsToday, riskScore, cooldownUntil, actorBlocked == 1, msgType), nil
}

// DecideCaps is the PURE cap decision — no DB, no side effects, and `now` is passed
// in (not read from the clock) so callers get deterministic results and the UTC
// day-rollover comparison can't go flaky in tests. It is the SINGLE source of
// cap-gate truth, shared by the queue-time gate (CheckCapsTx, which reads + applies
// risk decay first) and the read-only readiness matrix (EvaluateCaps, no decay) —
// so the gate and the matrix can never disagree on why an account is blocked.
func DecideCaps(now time.Time, caps models.BehaviourCaps, countersDay string, commentsToday, inboxToday, groupPostsToday, profilePostsToday int, riskScore float64, cooldownUntil time.Time, actorBlocked bool, msgType string) CapsDecision {
	now = now.UTC()
	// Verified-Actor block (P1b): an account caught logged into a different
	// Facebook identity than expected is denied ALL execution until an operator
	// clears it. Checked first — a hard integrity stop, not a pacing decision.
	if actorBlocked {
		return CapsDecision{Allowed: false, Reason: "actor_mismatch_blocked"}
	}
	if !cooldownUntil.IsZero() && now.Before(cooldownUntil.UTC()) {
		return CapsDecision{Allowed: false, Reason: "account_cooldown_active", CooldownUntil: cooldownUntil}
	}
	if caps.RiskScoreCeiling > 0 && riskScore >= caps.RiskScoreCeiling {
		return CapsDecision{Allowed: false, Reason: "risk_ceiling_exceeded", RiskScore: riskScore, RiskCeiling: caps.RiskScoreCeiling}
	}
	if col := counterColumnForAction(msgType); col != "" {
		cap := caps.CapForAction(msgType)
		if cap > 0 {
			counter := 0
			if countersDay == dbutil.UTCDayKey(now) {
				switch col {
				case "comments_today":
					counter = commentsToday
				case "inbox_today":
					counter = inboxToday
				case "group_posts_today":
					counter = groupPostsToday
				case "profile_posts_today":
					counter = profilePostsToday
				}
			}
			if counter >= cap {
				return CapsDecision{Allowed: false, Reason: "daily_limit_exceeded"}
			}
		}
	}
	return CapsDecision{Allowed: true, Reason: "ok"}
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
	return DecideCaps(time.Now().UTC(), caps, countersDay, commentsToday, inboxToday, groupPostsToday,
		profilePostsToday, riskScore, cooldownUntil, actorBlocked == 1, msgType), nil
}
