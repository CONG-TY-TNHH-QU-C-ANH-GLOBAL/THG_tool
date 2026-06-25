package outbound

import (
	"context"
	"fmt"
	"time"

	"github.com/thg/scraper/internal/models"
)

// ResetStaleExecuting returns abandoned executing rows to planned
// when their per-row lease has expired. Two paths:
//
//   - Primary (new rows): lease_expiry is non-NULL and in the past.
//     The lease was set at claim time via [Store.Claim]; once it
//     expires the row is fair game for a re-claim.
//   - Legacy (rows claimed before the lease column existed):
//     lease_expiry IS NULL. Falls back to the previous claimed_at +
//     staleAfter window so historical data still drains.
//
// Resetting CLEARS execution_id so the next claim issues a fresh
// token — any in-flight report from the abandoned attempt then rightly
// fails its execution_id CAS at finalize time and is rejected as
// stale, preventing the SW-restart-then-re-claim duplicate-comment
// bug class.
//
// PR-2 (V2 staged refactor): each row reset appends a 'reset'
// transition to execution_attempts (best-effort) so the audit trail
// reflects lease-eviction events. The UPDATE itself remains the
// authoritative state change.
func (s *Store) ResetStaleExecuting(orgID int64, staleAfter time.Duration) error {
	if orgID <= 0 {
		return nil
	}
	if staleAfter <= 0 {
		staleAfter = 10 * time.Minute
	}
	ctx := context.Background()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// Snapshot which rows we're about to reset, so we can attach
	// transition rows for each.
	staleClause := fmt.Sprintf("-%d seconds", int(staleAfter.Seconds()))
	rows, err := tx.QueryContext(ctx,
		`SELECT id, account_id, type, target_url, COALESCE(execution_id, '')
		 FROM outbound_messages
		 WHERE org_id = ?
		   AND execution_state = ?
		   AND (
		     (lease_expiry IS NOT NULL AND lease_expiry <= CURRENT_TIMESTAMP)
		     OR (lease_expiry IS NULL AND claimed_at IS NOT NULL AND claimed_at <= datetime('now', ?))
		   )`,
		orgID, models.ExecExecuting, staleClause,
	)
	if err != nil {
		return err
	}
	type staleRow struct {
		id          int64
		accountID   int64
		actionType  string
		targetURL   string
		executionID string
	}
	var stale []staleRow
	for rows.Next() {
		var r staleRow
		if rows.Scan(&r.id, &r.accountID, &r.actionType, &r.targetURL, &r.executionID) == nil {
			stale = append(stale, r)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if len(stale) == 0 {
		return tx.Commit()
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE outbound_messages
		 SET execution_state = ?, verification_outcome = NULL,
		     claimed_by = '', claimed_at = NULL,
		     execution_id = '', lease_expiry = NULL
		 WHERE org_id = ?
		   AND execution_state = ?
		   AND (
		     (lease_expiry IS NOT NULL AND lease_expiry <= CURRENT_TIMESTAMP)
		     OR (lease_expiry IS NULL AND claimed_at IS NOT NULL AND claimed_at <= datetime('now', ?))
		   )`,
		models.ExecPlanned,
		orgID, models.ExecExecuting,
		staleClause,
	)
	if err != nil {
		return err
	}

	for _, r := range stale {
		s.appendTransition(ctx, tx, transitionInput{
			OutboundID:     r.id,
			OrgID:          orgID,
			AccountID:      r.accountID,
			TargetURL:      r.targetURL,
			ActionType:     r.actionType,
			Attempt:        s.nextAttemptNumber(ctx, tx, orgID, r.id),
			TransitionType: TransitionReset,
			ExecutionID:    r.executionID, // capture the evicted token for audit
			ResultingState: models.ExecPlanned,
		})
	}
	return tx.Commit()
}
