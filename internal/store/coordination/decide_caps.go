// Domain: coordination (see internal/store/DOMAINS.md)
//
// Pure cap-decision policy — extracted from behaviour_caps_check.go
// (PR-2b) so the policy stays under the 200-line gate, separate from
// the DB wrappers (CheckCapsTx / EvaluateCaps) that feed it.
package coordination

import (
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/dbutil"
)

// CapsDecision is the coordination-domain primitive returned by the cap
// checks. It carries the result in a shape that does NOT reference any
// peer domain — the outbound layer's hook adapter converts this into
// outbound.GuardDecision at its boundary (see DOMAINS.md §2;
// [[feedback_no_bidirectional_domain_knowledge]]).
//
// Field semantics:
//
//   - Allowed: whether the action passes the cap check.
//   - Reason: short tag — "ok" | "assignment_paused_by_admin" |
//     "actor_mismatch_blocked" | "account_cooldown_active" |
//     "daily_limit_exceeded" | "risk_ceiling_exceeded". Stable strings
//     consumed by the outbound dedup/queue layer for telemetry.
//   - CooldownUntil: zero unless Reason == "account_cooldown_active".
type CapsDecision struct {
	Allowed       bool
	Reason        string
	CooldownUntil time.Time
	// RiskScore and RiskCeiling are populated ONLY when
	// Reason == "risk_ceiling_exceeded" so operator telemetry can show
	// "why was this blocked?" without a separate diagnostic round-trip.
	RiskScore   float64
	RiskCeiling float64
}

// DecideCaps is the PURE cap decision — no DB, no side effects, and `now` is passed
// in (not read from the clock) so callers get deterministic results and the UTC
// day-rollover comparison can't go flaky in tests. It is the SINGLE source of
// cap-gate truth, shared by the queue-time gate (CheckCapsTx, which reads + applies
// risk decay first) and the read-only readiness matrix (EvaluateCaps, no decay) —
// so the gate and the matrix can never disagree on why an account is blocked.
func DecideCaps(now time.Time, caps models.BehaviourCaps, countersDay string, commentsToday, inboxToday, groupPostsToday, profilePostsToday int, riskScore float64, cooldownUntil time.Time, actorBlocked bool, assignmentPaused bool, msgType string) CapsDecision {
	now = now.UTC()
	// Admin assignment pause (PR-2b): gate #0 — a deliberate operator
	// safety switch outranks every pacing/integrity signal. Typed reason
	// is stable for ledger/telemetry/UI consumers.
	if assignmentPaused {
		return CapsDecision{Allowed: false, Reason: "assignment_paused_by_admin"}
	}
	// Verified-Actor block (P1b): an account caught logged into a different
	// Facebook identity than expected is denied ALL execution until an operator
	// clears it. A hard integrity stop, not a pacing decision.
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
