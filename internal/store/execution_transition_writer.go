// Domain: coordination (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/thg/scraper/internal/store/dbutil"
)

// recordExecutionTransitionTx is the coordination-domain writer for
// execution_attempts row append. Decouple-2 (2026-05-21) moved this
// INSERT out of outbound/transition.go and exposed it as a hook so the
// canonical-writer rule for the coordination-owned execution_attempts
// table holds: only the coordination domain INSERTs into it.
//
// All input fields are primitives — the hook surface uses
// outbound.RecordTransitionInput at the call site but unpacks to
// primitives here. This keeps coordination free of any outbound import,
// which is the cycle-avoidance pre-work for Phase 5B extraction.
//
// Best-effort: failures are logged at warn-level; the caller (outbound's
// appendTransition) treats this as fire-and-forget so a transient ledger
// blip cannot revert a committed outbound CAS.
//
// tenant-ok: this function is the canonical writer for the
// execution_attempts table. It is called from outbound's queue/finalize
// path inside outbound's transaction (passed as tx). When coordination
// is extracted (Phase 5B), this function moves to
// internal/store/coordination/ and the hook closure in
// installOutboundHooks repoints at the new location.
func (s *Store) recordExecutionTransitionTx(
	ctx context.Context,
	tx *sql.Tx,
	outboundID, orgID, accountID int64,
	targetURL, actionType string,
	attempt int,
	status, outcome string,
	failureReason, evidenceJSON string,
	domVerified, networkVerified bool,
	transitionType, executionID string,
	resultingState, resultingOutcome string,
	leaseExpiry sql.NullTime,
) {
	var outcomeArg interface{}
	if resultingOutcome != "" {
		outcomeArg = resultingOutcome
	}
	var leaseArg interface{}
	if leaseExpiry.Valid {
		leaseArg = leaseExpiry.Time
	}
	evidence := evidenceJSON
	if evidence == "" {
		evidence = "{}"
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO execution_attempts
			(action_ledger_id, outbound_id, org_id, account_id, target_url,
			 action_type, attempt, status, outcome, failure_reason, evidence_json,
			 dom_verified, network_verified, started_at, finished_at,
			 transition_type, execution_id, resulting_state, resulting_outcome, lease_expiry)
		VALUES (0, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, CURRENT_TIMESTAMP, CASE WHEN ? = 'finalize' THEN CURRENT_TIMESTAMP ELSE NULL END,
			?, ?, ?, ?, ?)`,
		outboundID, orgID, accountID, targetURL,
		actionType, attempt, status,
		outcome, failureReason, evidence,
		dbutil.BoolToInt(domVerified), dbutil.BoolToInt(networkVerified),
		transitionType,
		transitionType, executionID, resultingState, outcomeArg, leaseArg,
	)
	if err != nil {
		slog.WarnContext(ctx, "coordination.recordExecutionTransitionTx: insert failed (best-effort, not load-bearing)",
			"outbound_id", outboundID, "org_id", orgID,
			"transition_type", transitionType, "error", err,
		)
	}
}
