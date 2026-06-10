package coordination

import (
	"context"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// Manual human verification (spec: specs/COMMENT_ASYNC_REVERIFY.md companion, Part A).
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

	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO comment_verification_audit
			(org_id, outbound_id, target_url, account_id, verified_by_user_id, source,
			 previous_outcome, new_effective_outcome, correction_ledger_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.OrgID, in.OutboundID, strings.TrimSpace(in.TargetURL), in.AccountID, in.VerifiedBy,
		models.HumanVerifySource, in.PreviousOutcome, LedgerOutcomeSucceeded, ledgerID,
	)

	return models.HumanVerifyResult{
		Corrected:           true,
		CorrectionLedgerID:  ledgerID,
		NewEffectiveOutcome: LedgerOutcomeSucceeded,
	}, nil
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
