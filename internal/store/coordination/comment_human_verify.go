package coordination

import (
	"context"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// Manual human verification (spec: specs/domains/facebook-sales-intelligence/features/comment-automation/technical.md companion, Part A).
// Coordination owns this because the correction is an action_ledger INSERT (topology §6.3)
// and must stay append-only (§6.4) — the old submitted_unverified row is never mutated.

// HumanVerifyInput carries the explicit fields the correction + audit need (the handler has
// already loaded + eligibility-checked the outbound).
type HumanVerifyInput struct {
	OrgID           int64
	OutboundID      int64
	TargetURL       string
	AccountID       int64
	VerifiedBy      int64
	PreviousOutcome string
}

// AppendHumanVerifyCorrection appends a 'succeeded'/'human_verified' correction for a
// manually-confirmed comment and records the audit trail. Idempotent: if a correction
// (human_verified OR reverified) already exists for this outbound it is a no-op — no
// duplicate ledger row.
func (s *Store) AppendHumanVerifyCorrection(ctx context.Context, in HumanVerifyInput) (models.HumanVerifyResult, error) {
	if in.OrgID <= 0 || in.OutboundID <= 0 {
		return models.HumanVerifyResult{}, fmt.Errorf("human verify requires org_id + outbound_id")
	}
	if existing := s.existingCommentCorrection(ctx, in.OrgID, in.OutboundID); existing > 0 {
		return models.HumanVerifyResult{
			AlreadyVerified:     true,
			CorrectionLedgerID:  existing,
			NewEffectiveOutcome: LedgerOutcomeSucceeded,
		}, nil
	}

	ledgerID, err := s.RecordActionLedger(ctx, ActionLedgerEntry{
		OrgID:      in.OrgID,
		ActionType: "comment",
		TargetURL:  strings.TrimSpace(in.TargetURL),
		AccountID:  in.AccountID,
		CreatedBy:  in.VerifiedBy,
		OutboundID: in.OutboundID,
		Outcome:    LedgerOutcomeSucceeded,
		Reason:     models.LedgerReasonHumanVerified,
	})
	if err != nil {
		return models.HumanVerifyResult{}, err
	}

	auditRes, _ := s.db.ExecContext(ctx,
		`INSERT INTO comment_verification_audit
			(org_id, outbound_id, target_url, account_id, verified_by_user_id, source,
			 previous_outcome, new_effective_outcome, correction_ledger_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.OrgID, in.OutboundID, strings.TrimSpace(in.TargetURL), in.AccountID, in.VerifiedBy,
		models.HumanVerifySource, in.PreviousOutcome, LedgerOutcomeSucceeded, ledgerID,
	)
	var auditID int64
	if auditRes != nil {
		auditID, _ = auditRes.LastInsertId()
	}

	return models.HumanVerifyResult{
		Corrected:           true,
		CorrectionLedgerID:  ledgerID,
		AuditID:             auditID,
		NewEffectiveOutcome: LedgerOutcomeSucceeded,
	}, nil
}

// CommentCorrectionsForOutbounds returns the latest succeeded correction (human_verified or
// reverified) per outbound id — so the dashboard can show the LATEST EFFECTIVE outcome
// instead of the stale (append-only, never-mutated) outbound verification_outcome.
func (s *Store) CommentCorrectionsForOutbounds(ctx context.Context, orgID int64, ids []int64) (map[int64]models.CommentCorrection, error) {
	out := map[int64]models.CommentCorrection{}
	if orgID <= 0 || len(ids) == 0 {
		return out, nil
	}
	ph := strings.Repeat("?,", len(ids))
	ph = ph[:len(ph)-1]
	args := make([]any, 0, len(ids)+1)
	args = append(args, orgID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT outbound_id, id, reason FROM action_ledger
		  WHERE org_id = ? AND action_type = 'comment' AND outcome = 'succeeded'
		    AND reason IN ('human_verified','reverified') AND outbound_id IN (`+ph+`)
		  ORDER BY performed_at ASC`,
		args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var outboundID, correctionID int64
		var reason string
		if err := rows.Scan(&outboundID, &correctionID, &reason); err != nil {
			return out, err
		}
		// ASC order → the last write per outbound_id wins (latest correction).
		out[outboundID] = models.CommentCorrection{CorrectionID: correctionID, Reason: reason, Outcome: LedgerOutcomeSucceeded}
	}
	return out, rows.Err()
}

// existingCommentCorrection returns the id of an existing succeeded correction
// (human_verified or reverified) for the outbound, 0 if none — the idempotency guard.
func (s *Store) existingCommentCorrection(ctx context.Context, orgID, outboundID int64) int64 {
	var id int64
	_ = s.db.QueryRowContext(ctx,
		`SELECT id FROM action_ledger
		  WHERE outbound_id = ? AND org_id = ? AND outcome = 'succeeded'
		    AND reason IN (?, ?)
		  ORDER BY performed_at DESC LIMIT 1`,
		outboundID, orgID, models.LedgerReasonHumanVerified, ReverifyCorrectionReason,
	).Scan(&id)
	return id
}
