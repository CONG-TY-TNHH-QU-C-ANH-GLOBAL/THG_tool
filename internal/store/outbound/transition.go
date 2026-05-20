package outbound

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/dbutil"
)

// transitionInput is the parameter struct for appendTransition. All
// fields are denormalized snapshots of the state AFTER the transition
// — the projection columns on outbound_messages should already match.
type transitionInput struct {
	OutboundID     int64
	OrgID          int64
	AccountID      int64
	TargetURL      string
	ActionType     string
	Attempt        int
	TransitionType TransitionType
	ExecutionID    string
	ResultingState models.ExecutionState
	// ResultingOutcome is empty for non-finalize transitions. SQLite
	// stores it as NULL via the nil pointer trick below.
	ResultingOutcome models.VerificationOutcome
	LeaseExpiry      sql.NullTime
	// Outcome / FailureReason / EvidenceJSON / DOMVerified are
	// forwarded into the legacy execution_attempts columns so existing
	// analytics queries (idx_execution_attempts_org_outcome) keep
	// working unchanged.
	Outcome         models.ExecutionOutcome
	FailureReason   string
	EvidenceJSON    string
	DOMVerified     bool
	NetworkVerified bool
}

// appendTransition writes one row to execution_attempts as part of
// the same tx as the outbound state-changing UPDATE.
//
// The INSERT is best-effort: an error here is LOGGED but never
// returned to the caller, so a transient ledger failure cannot revert
// a committed CAS. The row-level CAS on outbound_messages remains the
// authoritative concurrency control.
//
// tenant-ok: cross-domain projection (outbound -> coordination). The
// execution_attempts table is owned by the coordination domain
// (Phase 5 target). Outbound writes to it directly today as the
// append-only audit ledger — this will move to an injected hook when
// coordination is extracted.
func (s *Store) appendTransition(ctx context.Context, tx *sql.Tx, t transitionInput) {
	if t.OutboundID == 0 || t.OrgID == 0 {
		slog.WarnContext(ctx, "outbound.appendTransition: missing outbound_id or org_id",
			"outbound_id", t.OutboundID, "org_id", t.OrgID)
		return
	}
	if t.TransitionType == "" {
		t.TransitionType = TransitionFinalize
	}
	if t.Attempt <= 0 {
		t.Attempt = 1
	}

	var outcomeArg interface{}
	if t.ResultingOutcome != "" {
		outcomeArg = string(t.ResultingOutcome)
	}
	var leaseArg interface{}
	if t.LeaseExpiry.Valid {
		leaseArg = t.LeaseExpiry.Time
	}
	evidence := t.EvidenceJSON
	if evidence == "" {
		evidence = "{}"
	}
	status := models.AttemptDOMVerified
	if t.TransitionType != TransitionFinalize {
		status = models.AttemptQueued
	}
	if t.TransitionType == TransitionFinalize && !models.IsSuccessOutcome(t.Outcome) {
		status = models.AttemptFailed
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
		t.OutboundID, t.OrgID, t.AccountID, t.TargetURL,
		t.ActionType, t.Attempt, string(status),
		string(t.Outcome), t.FailureReason, evidence,
		dbutil.BoolToInt(t.DOMVerified), dbutil.BoolToInt(t.NetworkVerified),
		string(t.TransitionType),
		string(t.TransitionType), t.ExecutionID, string(t.ResultingState), outcomeArg, leaseArg,
	)
	if err != nil {
		slog.WarnContext(ctx, "outbound.appendTransition: ledger insert failed (best-effort, not load-bearing)",
			"outbound_id", t.OutboundID, "org_id", t.OrgID,
			"transition_type", t.TransitionType, "error", err,
		)
	}
}

// nextAttemptNumber returns the next attempt counter for an outbound
// row by counting existing execution_attempts rows + 1. Best-effort —
// returns 1 if the count fails (the ledger is not load-bearing).
// Tenant-scoped: filters by both outbound_id and org_id so a poisoned
// outbound_id pointing at another tenant's row cannot influence this
// tenant's attempt counter.
//
// tenant-ok: cross-domain projection (outbound -> coordination).
func (s *Store) nextAttemptNumber(ctx context.Context, tx *sql.Tx, orgID, outboundID int64) int {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(attempt), 0) + 1 FROM execution_attempts WHERE outbound_id = ? AND org_id = ?`,
		outboundID, orgID,
	).Scan(&n)
	if err != nil || n <= 0 {
		return 1
	}
	return n
}
