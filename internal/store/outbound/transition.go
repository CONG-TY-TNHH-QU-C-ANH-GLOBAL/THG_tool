package outbound

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/thg/scraper/internal/models"
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

// appendTransition routes the transition row through the
// [Hooks.RecordTransition] callback. The hook owner (coordination
// domain — today wired in outbound_aliases.go::installOutboundHooks)
// performs the actual INSERT into execution_attempts.
//
// Decouple-2 (2026-05-21) moved the SQL out of outbound to satisfy
// the cross-domain-write rule: execution_attempts is owned by
// coordination, so outbound MUST NOT INSERT into it directly. The
// hook indirection preserves the same tx (the closure runs inside
// outbound's queue/finalize tx) so atomicity is unchanged.
//
// Best-effort semantics preserved: hook failures are LOGGED inside
// the implementation, never returned to the outbound caller — the
// row-level CAS on outbound_messages remains the authoritative
// concurrency control.
//
// tenant-ok: cross-domain projection (outbound -> coordination).
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
	if s.hooks.RecordTransition == nil {
		// Defensive: an uninitialised hook would silently drop the audit
		// row. Match the BehaviourCheck pattern — emit a warn-level log
		// once per call instead of crashing the queue path.
		slog.WarnContext(ctx, "outbound.appendTransition: RecordTransition hook is nil; skipping ledger write",
			"outbound_id", t.OutboundID, "org_id", t.OrgID,
			"transition_type", t.TransitionType,
		)
		return
	}

	status := models.AttemptDOMVerified
	if t.TransitionType != TransitionFinalize {
		status = models.AttemptQueued
	}
	if t.TransitionType == TransitionFinalize && !models.IsSuccessOutcome(t.Outcome) {
		status = models.AttemptFailed
	}

	s.hooks.RecordTransition(ctx, tx, RecordTransitionInput{
		OutboundID:       t.OutboundID,
		OrgID:            t.OrgID,
		AccountID:        t.AccountID,
		TargetURL:        t.TargetURL,
		ActionType:       t.ActionType,
		Attempt:          t.Attempt,
		Status:           string(status),
		Outcome:          string(t.Outcome),
		FailureReason:    t.FailureReason,
		EvidenceJSON:     t.EvidenceJSON,
		DOMVerified:      t.DOMVerified,
		NetworkVerified:  t.NetworkVerified,
		TransitionType:   string(t.TransitionType),
		ExecutionID:      t.ExecutionID,
		ResultingState:   string(t.ResultingState),
		ResultingOutcome: string(t.ResultingOutcome),
		LeaseExpiry:      t.LeaseExpiry,
	})
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
