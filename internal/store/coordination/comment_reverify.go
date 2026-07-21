package coordination

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Async comment reverify queue + append-only correction (spec: specs/domains/facebook-sales-intelligence/features/comment-automation/technical.md,
// PR-A). Coordination owns this because the correction it produces is an action_ledger
// INSERT (topology §6.3 — ledger writes only here) and it must stay append-only (§6.4).

// Reverify outcomes (comment_reverify.outcome).
const (
	ReverifyPending  = "pending"
	ReverifyVerified = "verified"
	ReverifyNotFound = "not_found"
	ReverifyError    = "error"
	// ReverifyReason recorded on the appended correction ledger row.
	ReverifyCorrectionReason = "reverified"
)

// ReverifyClaimLease bounds how long a claimed-but-unreported job is considered in-flight.
// After this, the lease-aware claim re-offers it (the connector crashed before reporting) —
// so a job can never sit pending+claimed forever.
const ReverifyClaimLease = 5 * time.Minute

// ReverifyMaxClaimsWithoutAttempt self-heals against a connector running stale code that
// keeps claiming but never reports: after this many claims with attempted_at still NULL, the
// job is retired as error=claim_without_attempt instead of looping forever.
const ReverifyMaxClaimsWithoutAttempt = 3

// ReverifyReasonClaimWithoutAttempt is the self-heal reason — surfaces "the connector
// claimed but never processed" (almost always a stale/broken extension).
const ReverifyReasonClaimWithoutAttempt = "claim_without_attempt"

// ReverifyJob is one row of the reverify queue handed to the extension.
type ReverifyJob struct {
	ID         int64  `json:"id"`
	OrgID      int64  `json:"org_id"`
	OutboundID int64  `json:"outbound_id"`
	TargetURL  string `json:"target_url"`
	AccountID  int64  `json:"account_id"`
	CreatedBy  int64  `json:"created_by"`
	Content    string `json:"content"`
}

// ScheduleReverify enqueues a reverify for a submitted-unverified outbound. Idempotent:
// UNIQUE(outbound_id) means a second schedule for the same outbound is a no-op.
func (s *Store) ScheduleReverify(ctx context.Context, j ReverifyJob, scheduledFor time.Time) error {
	if j.OrgID <= 0 || j.OutboundID <= 0 || strings.TrimSpace(j.TargetURL) == "" {
		return fmt.Errorf("reverify requires org_id, outbound_id, target_url")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO comment_reverify
			(org_id, outbound_id, target_url, account_id, created_by, content, scheduled_for, outcome)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		j.OrgID, j.OutboundID, strings.TrimSpace(j.TargetURL), j.AccountID, j.CreatedBy,
		j.Content, scheduledFor.UTC().Format("2006-01-02 15:04:05"), ReverifyPending,
	)
	return err
}

// ClaimDueReverifies returns up to `limit` pending reverify jobs whose schedule has
// arrived, scoped to the org (and to a specific account when accountID>0 — a connector can
// only re-check posts as the account it is logged in as), stamping claimed_at so a second
// poller does not double-claim them.
func (s *Store) ClaimDueReverifies(ctx context.Context, orgID, accountID, tokenID int64, now time.Time, limit int) ([]ReverifyJob, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("reverify claim requires org_id")
	}
	if limit <= 0 {
		limit = 20
	}
	// Self-heal FIRST: a job claimed too many times with attempted_at still NULL means the
	// connector is claiming but never processing (stale/broken extension). Retire it as
	// error so it never loops forever — and so the operator sees the cause.
	_, _ = s.db.ExecContext(ctx,
		`UPDATE comment_reverify
		    SET outcome = ?, reason = ?, attempted_at = CURRENT_TIMESTAMP
		  WHERE org_id = ? AND outcome = ? AND attempted_at IS NULL AND claim_count >= ?`,
		ReverifyError, ReverifyReasonClaimWithoutAttempt, orgID, ReverifyPending, ReverifyMaxClaimsWithoutAttempt,
	)

	// Lease-aware: claim pending jobs that are unclaimed OR whose claim lease expired (the
	// connector crashed before reporting), and that have not exhausted the claim budget.
	leaseCutoff := now.Add(-ReverifyClaimLease).UTC().Format("2006-01-02 15:04:05")
	query := `SELECT id, org_id, outbound_id, target_url, account_id, created_by, COALESCE(content,'')
	   FROM comment_reverify
	  WHERE outcome = ? AND scheduled_for <= ? AND org_id = ?
	    AND (claimed_at IS NULL OR claimed_at <= ?)
	    AND claim_count < ?`
	args := []any{ReverifyPending, now.UTC().Format("2006-01-02 15:04:05"), orgID, leaseCutoff, ReverifyMaxClaimsWithoutAttempt}
	if accountID > 0 {
		query += ` AND account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY scheduled_for ASC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []ReverifyJob
	for rows.Next() {
		var j ReverifyJob
		if err := rows.Scan(&j.ID, &j.OrgID, &j.OutboundID, &j.TargetURL, &j.AccountID, &j.CreatedBy, &j.Content); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, j := range jobs {
		// Record claimed_at + WHO claimed (agent_token id) + bump the claim counter so the
		// self-heal above can retire a job the connector keeps claiming but never attempting.
		_, _ = s.db.ExecContext(ctx,
			`UPDATE comment_reverify
			    SET claimed_at = CURRENT_TIMESTAMP, claim_count = claim_count + 1, claimed_by_token_id = ?
			  WHERE id = ?`,
			tokenID, j.ID)
	}
	return jobs, nil
}

// RecordReverifyOutcome stamps the terminal outcome (verified | not_found | error) + reason
// and attempted_at. The comment_reverify row is mutable tracking — NOT the append-only
// ledger; the verified correction itself goes to action_ledger via AppendReverifyCorrection.
func (s *Store) RecordReverifyOutcome(ctx context.Context, id int64, outcome, reason string) error {
	if id <= 0 {
		return fmt.Errorf("reverify id required")
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE comment_reverify
		    SET outcome = ?, reason = ?, attempted_at = CURRENT_TIMESTAMP
		  WHERE id = ?`,
		outcome, reason, id,
	)
	return err
}

// AppendReverifyCorrection appends a VERIFIED 'succeeded' comment touch to action_ledger for
// the original outbound — the append-only correction submitted_unverified → succeeded. The
// old submitted_unverified row is never mutated; the engagement projection (action_type=
// 'comment', outcome='succeeded') now counts the lead as a hard verified touch.
func (s *Store) AppendReverifyCorrection(ctx context.Context, j ReverifyJob) (int64, error) {
	if j.OrgID <= 0 || strings.TrimSpace(j.TargetURL) == "" {
		return 0, fmt.Errorf("reverify correction requires org_id + target_url")
	}
	return s.RecordActionLedger(ctx, ActionLedgerEntry{
		OrgID:      j.OrgID,
		ActionType: "comment",
		TargetURL:  strings.TrimSpace(j.TargetURL),
		AccountID:  j.AccountID,
		CreatedBy:  j.CreatedBy,
		OutboundID: j.OutboundID,
		Outcome:    LedgerOutcomeSucceeded,
		Reason:     ReverifyCorrectionReason,
	})
}
