// Domain: coordination (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
	"github.com/thg/scraper/internal/store/outbound"
)

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
// When coordination is extracted as its own subpackage (Phase 5), this
// function moves into `internal/store/coordination/` and the hook
// continues to point at it via the same closure indirection.
//
// tenant-ok: cross-domain projection (outbound -> coordination). The
// account_runtime_state table is owned by the coordination domain;
// outbound queries it only via this injected hook.
func (s *Store) checkBehaviourCapsTx(tx *sql.Tx, accountID int64, msgType string) (outbound.GuardDecision, error) {
	caps, _, err := s.ResolveAccountCaps(context.Background(), accountID)
	if err != nil {
		return outbound.GuardDecision{}, err
	}

	// Single round-trip: read every column the cap decision needs in
	// one SELECT, then apply day-rollover + cap check in Go.
	var (
		countersDay                                                    string
		commentsToday, inboxToday, groupPostsToday, profilePostsToday int
		riskScore                                                      float64
		cooldownUntilStr                                               string
	)
	err = tx.QueryRow(
		`SELECT counters_day, comments_today, inbox_today, group_posts_today,
		        profile_posts_today, risk_score, COALESCE(cooldown_until,'')
		   FROM account_runtime_state
		  WHERE account_id = ?`, accountID,
	).Scan(&countersDay, &commentsToday, &inboxToday, &groupPostsToday,
		&profilePostsToday, &riskScore, &cooldownUntilStr)
	if err != nil && err != sql.ErrNoRows {
		return outbound.GuardDecision{}, err
	}

	if cooldownUntilStr != "" {
		until := dbutil.ParseSQLiteTime(cooldownUntilStr)
		if !until.IsZero() && time.Now().UTC().Before(until.UTC()) {
			return outbound.GuardDecision{
				Allowed:        false,
				Reason:         "account_cooldown_active",
				LastOutboundAt: until,
			}, nil
		}
	}

	if caps.RiskScoreCeiling > 0 && riskScore >= caps.RiskScoreCeiling {
		return outbound.GuardDecision{Allowed: false, Reason: "risk_ceiling_exceeded"}, nil
	}

	if col := counterColumnForAction(msgType); col != "" {
		cap := caps.CapForAction(msgType)
		if cap > 0 {
			counter := 0
			if countersDay == dbutil.UTCDayKey(time.Now()) {
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
				return outbound.GuardDecision{Allowed: false, Reason: "daily_limit_exceeded"}, nil
			}
		}
	}

	return outbound.GuardDecision{Allowed: true, Reason: "ok"}, nil
}
