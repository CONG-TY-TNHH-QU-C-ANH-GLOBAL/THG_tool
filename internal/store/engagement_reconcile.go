package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/thg/scraper/internal/models"
)

// ReconcileEngagementReport summarises what the reconciliation pass
// changed. Exposed so callers (CLI tool, admin endpoint, scheduled
// job) can render an audit summary without re-running the SQL.
type ReconcileEngagementReport struct {
	// LedgerScanned is the count of action_ledger rows we evaluated.
	LedgerScanned int
	// LedgerCorrected is the count of action_ledger rows we flipped
	// from outcome='succeeded' to outcome='failed' because the
	// downstream execution_attempts row contradicts the "succeeded"
	// claim.
	LedgerCorrected int
	// LedgerKept is the count of rows where the ledger claim and the
	// attempt verdict agree — no change applied.
	LedgerKept int
	// LedgerOrphaned is the count of ledger rows with no
	// corresponding execution_attempts row. We do NOT touch these —
	// historical rows from before execution_attempts existed have
	// no second source of truth.
	LedgerOrphaned int
}

// ReconcileEngagement repairs historical false-positive "touched" states
// on the action_ledger table.
//
// THE PROBLEM (project goal, May-2026)
// ====================================
// Before commit 185d1d3 (backend EnforceTargetIdentity invariant), an
// outbound action could finish with verifier outcome dom_verified
// even though the comment landed on the wrong post (the May-2026
// route-mismatch incident, commit 1b93629 root cause). When
// finalizeOutbound called MarkActionLedgerOutcomeByOutbound it
// stamped outcome='succeeded' onto action_ledger via
// LedgerOutcomeAlias, and the lead engagement projection then
// surfaced that ledger row as a "touch". After the fix the
// engagement projection filters out non-succeeded rows, but
// historical ledger rows that were INCORRECTLY marked succeeded
// still claim to be verified touches.
//
// THE FIX
// =======
// For each action_ledger row with outcome='succeeded', look up the
// most recent execution_attempts row for the same outbound_id and
// check its outcome against models.IsSuccessOutcome. If the attempt
// outcome is NOT a success class (e.g. redirected_feed, context_drift,
// blocked, shadow_rejected), the ledger row is misclassified: flip
// it to outcome='failed' with reason=the actual attempt outcome so
// the audit trail preserves what really happened.
//
// SAFE BY DESIGN
// ==============
//   - Only DOWNGRADES (succeeded → failed). Never promotes.
//   - Only touches rows that have a contradicting attempt; ledger rows
//     without any execution_attempt are left alone (orphaned counter
//     tracks them).
//   - Idempotent — running twice produces the same end state.
//   - Org-scoped — pass orgID==0 to reconcile everything; otherwise
//     scoped to one tenant.
//   - Read-only fast path: returns early when nothing to fix.
func (s *Store) ReconcileEngagement(ctx context.Context, orgID int64) (*ReconcileEngagementReport, error) {
	rep := &ReconcileEngagementReport{}

	// Pull every (action_ledger, latest execution_attempt) pair where
	// the ledger says succeeded but the attempt has a different
	// outcome. The subquery picks the latest attempt per outbound_id
	// so a successful retry after a failed attempt is preserved.
	query := `
		WITH latest_attempt AS (
			SELECT outbound_id, outcome, failure_reason
			  FROM execution_attempts ea
			 WHERE ea.id = (
			     SELECT MAX(id) FROM execution_attempts WHERE outbound_id = ea.outbound_id
			 )
		)
		SELECT al.id, al.org_id, al.outbound_id, al.outcome,
		       la.outcome, COALESCE(la.failure_reason, '')
		  FROM action_ledger al
		  LEFT JOIN latest_attempt la ON la.outbound_id = al.outbound_id
		 WHERE al.outcome = ?`
	args := []any{LedgerOutcomeSucceeded}
	if orgID > 0 {
		query += ` AND al.org_id = ?`
		args = append(args, orgID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("reconcile: scan succeeded ledgers: %w", err)
	}

	type pendingFix struct {
		ledgerID      int64
		orgID         int64
		attemptOutcome string
		failureReason string
	}
	var fixes []pendingFix

	for rows.Next() {
		var (
			ledgerID, ledgerOrgID, outboundID int64
			ledgerOutcome                      string
			attemptOutcome                     *string
			failureReason                      string
		)
		if err := rows.Scan(&ledgerID, &ledgerOrgID, &outboundID, &ledgerOutcome, &attemptOutcome, &failureReason); err != nil {
			rows.Close()
			return nil, fmt.Errorf("reconcile: scan row: %w", err)
		}
		rep.LedgerScanned++

		if attemptOutcome == nil || *attemptOutcome == "" {
			// No execution_attempt row for this ledger — pre-Step 3 row
			// with no second source of truth. Leave it alone; the
			// orphaned counter tells the operator how much historical
			// data we cannot independently verify.
			rep.LedgerOrphaned++
			continue
		}

		if models.IsSuccessOutcome(models.ExecutionOutcome(*attemptOutcome)) {
			// Attempt confirms the success — keep the ledger row.
			rep.LedgerKept++
			continue
		}

		// Mismatch: ledger says succeeded, attempt says otherwise.
		// Queue the fix; apply outside the rows iterator so the cursor
		// doesn't fight the UPDATE.
		fixes = append(fixes, pendingFix{
			ledgerID:       ledgerID,
			orgID:          ledgerOrgID,
			attemptOutcome: *attemptOutcome,
			failureReason:  failureReason,
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reconcile: iterator: %w", err)
	}

	if len(fixes) == 0 {
		return rep, nil
	}

	for _, f := range fixes {
		reason := f.attemptOutcome
		if f.failureReason != "" && f.failureReason != f.attemptOutcome {
			reason = f.attemptOutcome + ":" + f.failureReason
		}
		_, err := s.db.ExecContext(ctx,
			`UPDATE action_ledger SET outcome = ?, reason = ? WHERE id = ? AND outcome = ?`,
			LedgerOutcomeFailed, "reconciled:"+reason, f.ledgerID, LedgerOutcomeSucceeded,
		)
		if err != nil {
			slog.WarnContext(ctx, "reconcile: update failed",
				"ledger_id", f.ledgerID, "org_id", f.orgID, "error", err)
			continue
		}
		rep.LedgerCorrected++
	}
	slog.InfoContext(ctx, "reconcile: completed",
		"event", "engagement.reconcile",
		"org_id", orgID,
		"scanned", rep.LedgerScanned,
		"corrected", rep.LedgerCorrected,
		"kept", rep.LedgerKept,
		"orphaned", rep.LedgerOrphaned,
	)
	return rep, nil
}
