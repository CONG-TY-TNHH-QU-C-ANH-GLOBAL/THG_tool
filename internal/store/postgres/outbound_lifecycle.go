package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime/events"
	"github.com/thg/scraper/internal/store/outbound"
)

// newExecutionID generates the per-claim idempotency token. Mirrors the
// generator in internal/store/outbound (16 crypto-random bytes, hex, "exec_"
// prefix) so tokens are indistinguishable across backends. Duplicated rather
// than exported from outbound to keep this foundation package self-contained.
func newExecutionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("exec-fallback-%d", time.Now().UnixNano())
	}
	return "exec_" + hex.EncodeToString(b[:])
}

// ClaimPlannedOutboundForOrg atomically moves one planned row to executing,
// stamping a fresh execution_id + lease_expiry under a row-level CAS gated on
// (id, org_id, execution_state='planned'). Returns sql.ErrNoRows when no
// planned row matched (already claimed / wrong tenant / missing), preserving
// the SQLite Store.Claim contract. The by-id CAS is the concurrency primitive;
// FOR UPDATE SKIP LOCKED is reserved for a future select-and-claim path and is
// not needed for this exact-parity by-id claim.
//
// The OutboundClaimed telemetry event is emitted on success, matching the
// SQLite path (PR12 parity). The best-effort execution_attempts 'claim' audit
// row is NOT written here — that table is coordination-owned and hook-wired at
// the composition root, not the outbound storage layer (see package doc).
func (s *OutboundStore) ClaimPlannedOutboundForOrg(orgID, id int64, workerID string, leaseDuration time.Duration) (*outbound.ClaimResult, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "chrome-extension"
	}
	if leaseDuration <= 0 {
		leaseDuration = outbound.DefaultLease
	}
	execID := newExecutionID()
	leaseExpiry := time.Now().UTC().Add(leaseDuration)

	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx,
		`UPDATE outbound_messages
		 SET execution_state = $1, claimed_by = $2, claimed_at = NOW(),
		     execution_id = $3, lease_expiry = $4
		 WHERE id = $5 AND org_id = $6 AND execution_state = $7`,
		string(models.ExecExecuting), workerID, execID, leaseExpiry,
		id, orgID, string(models.ExecPlanned),
	)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, sql.ErrNoRows
	}

	// Best-effort projection for the telemetry event below (mirrors the
	// SQLite claim path). Errors are ignored — the authoritative state
	// change is the CAS above; telemetry must never fail the claim.
	var actionType, targetURL string
	var accountID int64
	_ = tx.QueryRow(ctx,
		`SELECT type, account_id, target_url FROM outbound_messages WHERE id = $1 AND org_id = $2`,
		id, orgID,
	).Scan(&actionType, &accountID, &targetURL)

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Typed event: claim succeeded — parity with internal/store/outbound's
	// OutboundClaimed emission (closes the "did the queued row reach the
	// executor?" diagnostic gap). Telemetry only; no durable write.
	events.Info(ctx, events.OutboundClaimed,
		events.FieldOrgID, orgID,
		events.FieldOutboundID, id,
		events.FieldAccountID, accountID,
		events.FieldActionType, actionType,
		events.FieldTargetURL, targetURL,
		"worker_id", workerID,
		"execution_id", execID,
	)

	return &outbound.ClaimResult{ExecutionID: execID, LeaseExpiry: leaseExpiry}, nil
}

// FinalizeOutboundAttempt is the terminal-state CAS gated on the execution_id
// token. Mirrors internal/store/outbound.Store.Finalize exactly: first report
// with the current execution_id (or legacy empty token) flips the row to
// terminal and stamps verification_outcome (clearing the lease); a replayed or
// stale report returns finalized=false plus the current state read back in the
// same transaction.
func (s *OutboundStore) FinalizeOutboundAttempt(
	ctx context.Context,
	orgID, id int64,
	executionID string,
	terminalState models.ExecutionState,
	verificationOutcome models.VerificationOutcome,
) (finalized bool, currentState models.ExecutionState, currentOutcome models.VerificationOutcome, currentExecID string, err error) {
	if terminalState != models.ExecFinished && terminalState != models.ExecExpired {
		return false, "", "", "", fmt.Errorf("postgres.FinalizeOutboundAttempt: terminalState must be finished or expired, got %q", terminalState)
	}
	// Expired rows never observed anything — NULL is the truthful outcome.
	if terminalState == models.ExecExpired {
		verificationOutcome = ""
	}

	tx, txErr := s.pool.Begin(ctx)
	if txErr != nil {
		return false, "", "", "", txErr
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	const cas = `
		UPDATE outbound_messages
		SET execution_state = $1,
		    verification_outcome = $2,
		    sent_at = CASE WHEN $3 = 'verified_success' THEN NOW() ELSE sent_at END,
		    claimed_by = '',
		    claimed_at = NULL,
		    lease_expiry = NULL
		WHERE id = $4 AND org_id = $5 AND execution_state = $6
		  AND (execution_id = '' OR execution_id = $7)`
	var outcomeArg any
	if verificationOutcome != "" {
		outcomeArg = string(verificationOutcome)
	}
	tag, execErr := tx.Exec(ctx, cas,
		string(terminalState), outcomeArg, string(verificationOutcome),
		id, orgID, string(models.ExecExecuting), executionID,
	)
	if execErr != nil {
		return false, "", "", "", execErr
	}
	if tag.RowsAffected() > 0 {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return false, "", "", "", commitErr
		}
		return true, terminalState, verificationOutcome, executionID, nil
	}

	// CAS did not finalize — disambiguate by reading the current row.
	var rowState, rowOutcome string
	scanErr := tx.QueryRow(ctx,
		`SELECT execution_state, COALESCE(verification_outcome, ''), COALESCE(execution_id, '')
		 FROM outbound_messages WHERE id = $1 AND org_id = $2`,
		id, orgID,
	).Scan(&rowState, &rowOutcome, &currentExecID)
	if scanErr != nil {
		return false, "", "", "", scanErr
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return false, "", "", "", commitErr
	}
	return false, models.ExecutionState(rowState), models.VerificationOutcome(rowOutcome), currentExecID, nil
}
