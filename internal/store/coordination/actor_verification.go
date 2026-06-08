// Domain: coordination (see internal/store/DOMAINS.md)
package coordination

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/dbutil"
)

// ClassifyActorVerdict is the deterministic Verified-Actor classifier
// (specs/COMMENT_INTELLIGENCE_PIPELINE.md §7b). It compares the account's
// EXPECTED Facebook identity (accounts.fb_user_id) against the ACTUAL
// c_user the executor observed on the page at finalize.
//
//   - both present and equal      → verified
//   - both present and different  → mismatch  (blocks the account)
//   - either missing              → unknown   (cannot verify; no block)
//
// Pure function — no I/O — so the gate, finalize path, and tests share
// one source of truth.
func ClassifyActorVerdict(expectedFBUserID, actualFBUserID string) string {
	e := strings.TrimSpace(expectedFBUserID)
	a := strings.TrimSpace(actualFBUserID)
	if e == "" || a == "" {
		return models.ActorVerdictUnknown
	}
	if e == a {
		return models.ActorVerdictVerified
	}
	return models.ActorVerdictMismatch
}

// MarkAttemptActorVerification stamps the Verified-Actor verdict onto the
// per-attempt audit row. This is APPEND-ONLY in spirit: each attempt is a
// distinct row finalized exactly once; the action_ledger is NEVER mutated
// for the verdict. Best-effort — a verdict write must not break finalize.
func (s *Store) MarkAttemptActorVerification(ctx context.Context, attemptID int64, expectedFBUserID, actualFBUserID, verdict string) error {
	if attemptID <= 0 {
		return fmt.Errorf("actor verification: attempt_id required")
	}
	// tenant-ok: attemptID is the issuance token from BeginExecutionAttempt,
	// never exposed across tenants (same rationale as FinishExecutionAttempt).
	_, err := s.db.ExecContext(ctx,
		`UPDATE execution_attempts
		    SET expected_fb_user_id = ?, actual_fb_user_id = ?, actor_verdict = ?
		  WHERE id = ?`,
		strings.TrimSpace(expectedFBUserID), strings.TrimSpace(actualFBUserID),
		strings.TrimSpace(verdict), attemptID,
	)
	return err
}

// RecordAccountActorVerdict upserts the account's current Verified-Actor
// state. When block is true (a mismatch), it sets the explicit actor_blocked
// gate field that CheckCapsTx denies on — the block PERSISTS until an
// operator clears it (it is not a timed cooldown). A verified/unknown verdict
// updates the last-seen fields but NEVER auto-clears an existing block; only
// [ClearActorBlock] does that.
//
// Counter columns are untouched (counters_day seeded only on first insert).
func (s *Store) RecordAccountActorVerdict(ctx context.Context, orgID, accountID int64, verdict, actualFBUserID, blockReason string, block bool) error {
	if orgID <= 0 || accountID <= 0 {
		return fmt.Errorf("actor verdict: org_id and account_id required")
	}
	blockedInt := 0
	var blockedAt any = nil
	if block {
		blockedInt = 1
		blockedAt = time.Now().UTC().Format("2006-01-02 15:04:05")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO account_runtime_state
			(account_id, org_id, counters_day, last_actor_verdict, last_actual_fb_user_id,
			 actor_blocked, actor_block_reason, actor_blocked_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(account_id) DO UPDATE SET
			org_id                 = excluded.org_id,
			last_actor_verdict     = excluded.last_actor_verdict,
			last_actual_fb_user_id = excluded.last_actual_fb_user_id,
			actor_blocked          = CASE WHEN ? = 1 THEN 1 ELSE account_runtime_state.actor_blocked END,
			actor_block_reason     = CASE WHEN ? = 1 THEN excluded.actor_block_reason ELSE account_runtime_state.actor_block_reason END,
			actor_blocked_at       = CASE WHEN ? = 1 THEN excluded.actor_blocked_at ELSE account_runtime_state.actor_blocked_at END,
			updated_at             = CURRENT_TIMESTAMP`,
		accountID, orgID, dbutil.UTCDayKey(time.Now()),
		strings.TrimSpace(verdict), strings.TrimSpace(actualFBUserID),
		blockedInt, strings.TrimSpace(blockReason), blockedAt,
		blockedInt, blockedInt, blockedInt,
	)
	return err
}

// ClearActorBlock is the operator override: it lifts a Verified-Actor block
// on one account so it can auto-execute again. Tenant-scoped. Idempotent —
// clearing an unblocked account is a no-op.
func (s *Store) ClearActorBlock(ctx context.Context, orgID, accountID int64) error {
	if orgID <= 0 || accountID <= 0 {
		return fmt.Errorf("clear actor block: org_id and account_id required")
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE account_runtime_state
		    SET actor_blocked = 0, actor_block_reason = '', actor_blocked_at = NULL,
		        updated_at = CURRENT_TIMESTAMP
		  WHERE account_id = ? AND org_id = ?`,
		accountID, orgID,
	)
	return err
}

// AccountActorStatesForOrg returns account_id → [models.ActorState] for the
// org's accounts that have any Verified-Actor state recorded. The API layer
// merges this into the actors projection (CommentingView / Agent Decision
// Inspector). Accounts with no row simply have no entry (treated as unknown,
// not blocked).
func (s *Store) AccountActorStatesForOrg(ctx context.Context, orgID int64) (map[int64]models.ActorState, error) {
	out := make(map[int64]models.ActorState)
	if orgID <= 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT account_id, COALESCE(last_actor_verdict,''), COALESCE(actor_blocked,0),
		        COALESCE(actor_block_reason,'')
		   FROM account_runtime_state WHERE org_id = ?`, orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID int64
		var verdict, reason string
		var blocked int
		if err := rows.Scan(&accountID, &verdict, &blocked, &reason); err != nil {
			continue
		}
		if verdict == "" && blocked == 0 {
			continue
		}
		out[accountID] = models.ActorState{
			Verdict:     verdict,
			Blocked:     blocked == 1,
			BlockReason: reason,
		}
	}
	return out, rows.Err()
}
