package outbound

import (
	"context"
	"fmt"

	"github.com/thg/scraper/internal/models"
)

// Finalize is the terminal-state CAS the agent callback (/sent and
// /failed) goes through. It encodes the execution_id-gated idempotency
// invariant and writes the dual-column state (execution_state +
// verification_outcome) in one atomic UPDATE.
//
// CAS guarantees:
//
//   - First report with the row's current execution_id wins → state
//     flips to terminal (ExecFinished or ExecExpired), verification
//     outcome stamped, lease_expiry cleared. Returns finalized=true.
//   - Replayed report with the SAME execution_id when the row is
//     already terminal → returns finalized=false + the current state.
//     Handlers should treat this as success-equivalent (the work
//     already landed) — the duplicate side effects (engagement event /
//     execution_attempts / risk signal) must NOT be replayed.
//   - Report carrying a DIFFERENT execution_id than the row's current
//     value → returns finalized=false + current state. Handlers should
//     409 the request — the original execution was reset by
//     [Store.ResetStaleExecuting] and re-claimed; the caller's report
//     is stale.
//   - Empty executionID in the request body is allowed for backward
//     compatibility with legacy extensions: the CAS treats it as a
//     state-only check.
//
// terminalState must be ExecFinished or ExecExpired. For ExecFinished
// the verificationOutcome captures the post-DOM classification (or
// VerifExecutionFailed if the executor surfaced no observation). For
// ExecExpired the verificationOutcome is ignored — the row never
// reached an observable state.
func (s *Store) Finalize(
	ctx context.Context,
	orgID, id int64,
	executionID string,
	terminalState models.ExecutionState,
	verificationOutcome models.VerificationOutcome,
) (finalized bool, currentState models.ExecutionState, currentOutcome models.VerificationOutcome, currentExecID string, err error) {
	if terminalState != models.ExecFinished && terminalState != models.ExecExpired {
		return false, "", "", "", fmt.Errorf("outbound.Finalize: terminalState must be finished or expired, got %q", terminalState)
	}
	// Defensive: clear verification_outcome for expired state — the row
	// never observed anything, NULL is the truthful value.
	if terminalState == models.ExecExpired {
		verificationOutcome = ""
	}
	tx, txErr := s.db.BeginTx(ctx, nil)
	if txErr != nil {
		return false, "", "", "", txErr
	}
	defer tx.Rollback() //nolint:errcheck

	const cas = `
		UPDATE outbound_messages
		SET execution_state = ?,
		    verification_outcome = ?,
		    sent_at = CASE WHEN ? = 'verified_success' THEN CURRENT_TIMESTAMP ELSE sent_at END,
		    claimed_by = '',
		    claimed_at = NULL,
		    lease_expiry = NULL
		WHERE id = ? AND org_id = ? AND execution_state = ?
		  AND (execution_id = '' OR execution_id = ?)`
	var outcomeArg interface{}
	if verificationOutcome == "" {
		outcomeArg = nil
	} else {
		outcomeArg = string(verificationOutcome)
	}
	res, execErr := tx.ExecContext(ctx, cas,
		string(terminalState), outcomeArg,
		string(verificationOutcome),
		id, orgID, models.ExecExecuting, executionID,
	)
	if execErr != nil {
		return false, "", "", "", execErr
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		var actionType string
		var accountID int64
		var targetURL string
		_ = tx.QueryRowContext(ctx,
			`SELECT type, account_id, target_url FROM outbound_messages WHERE id = ? AND org_id = ?`,
			id, orgID,
		).Scan(&actionType, &accountID, &targetURL)

		s.appendTransition(ctx, tx, transitionInput{
			OutboundID:       id,
			OrgID:            orgID,
			AccountID:        accountID,
			TargetURL:        targetURL,
			ActionType:       actionType,
			Attempt:          s.nextAttemptNumber(ctx, tx, orgID, id),
			TransitionType:   TransitionFinalize,
			ExecutionID:      executionID,
			ResultingState:   terminalState,
			ResultingOutcome: verificationOutcome,
		})
		if commitErr := tx.Commit(); commitErr != nil {
			return false, "", "", "", commitErr
		}
		return true, terminalState, verificationOutcome, executionID, nil
	}

	// CAS did not finalize. Disambiguate by reading the current row.
	row := tx.QueryRowContext(ctx,
		`SELECT execution_state, COALESCE(verification_outcome,''), COALESCE(execution_id,'')
		   FROM outbound_messages WHERE id = ? AND org_id = ?`,
		id, orgID,
	)
	var rowState, rowOutcome string
	if scanErr := row.Scan(&rowState, &rowOutcome, &currentExecID); scanErr != nil {
		return false, "", "", "", scanErr
	}
	if commitErr := tx.Commit(); commitErr != nil {
		return false, "", "", "", commitErr
	}
	return false, models.ExecutionState(rowState), models.VerificationOutcome(rowOutcome), currentExecID, nil
}
