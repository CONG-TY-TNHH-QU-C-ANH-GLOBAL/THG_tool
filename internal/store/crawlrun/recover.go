package crawlrun

import (
	"context"
	"database/sql"
	"errors"
)

// RecoverDispatchFailure fails a claimed run whose command dispatch failed and
// creates (or reuses) its single queued retry, in one transaction. Ordering is
// integrity-critical: the parent must leave 'running' BEFORE the retry inserts,
// or ux_fb_crawl_runs_one_open_source would reject the retry and wedge the
// source. The transaction commits only when it actually recovers a running
// attempt; stale, already-recovered, and wrong-status calls mutate nothing.
func (s *Store) RecoverDispatchFailure(ctx context.Context, in RecoverDispatchFailureInput) (RecoverDispatchFailureOutcome, error) {
	if err := s.requirePostgres(); err != nil {
		return RecoverDispatchFailureOutcome{}, err
	}
	if !in.Fence.valid() || in.ExpectedAccountID <= 0 {
		return RecoverDispatchFailureOutcome{Result: RecoverStaleAttempt}, nil
	}
	reason := in.ReasonCode
	if reason == "" {
		reason = dispatchFailedReason
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RecoverDispatchFailureOutcome{}, err
	}
	defer tx.Rollback()

	parent, err := lockParent(ctx, tx, in.Fence.OrgID, in.Fence.RunID)
	if errors.Is(err, sql.ErrNoRows) {
		return RecoverDispatchFailureOutcome{Result: RecoverStaleAttempt}, nil
	}
	if err != nil {
		return RecoverDispatchFailureOutcome{}, err
	}

	outcome, err := classifyAndRecover(ctx, tx, in, parent, reason)
	if err != nil {
		return RecoverDispatchFailureOutcome{}, err
	}
	if outcome.Result == RecoverRecovered {
		if err := tx.Commit(); err != nil {
			return RecoverDispatchFailureOutcome{}, err
		}
	}
	return outcome, nil
}

type parentRun struct {
	campaignID     int64
	sourceID       int64
	accountID      sql.NullInt64
	status         string
	attempt        int
	exitReasonCode string
}

func lockParent(ctx context.Context, tx *sql.Tx, orgID, runID int64) (parentRun, error) {
	var p parentRun
	err := tx.QueryRowContext(ctx,
		`SELECT campaign_id, source_id, account_id, status, attempt, exit_reason_code
		 FROM facebook_crawl_runs
		 WHERE org_id = $1 AND id = $2
		 FOR UPDATE`,
		orgID, runID).Scan(&p.campaignID, &p.sourceID, &p.accountID, &p.status, &p.attempt, &p.exitReasonCode)
	return p, err
}

func classifyAndRecover(ctx context.Context, tx *sql.Tx, in RecoverDispatchFailureInput, p parentRun, reason string) (RecoverDispatchFailureOutcome, error) {
	fenceMatches := p.attempt == in.Fence.Attempt &&
		p.accountID.Valid && p.accountID.Int64 == in.ExpectedAccountID

	if p.status != "running" {
		// Idempotent repeat: a prior recovery already failed this exact attempt
		// with this reason, so its retry must exist — find and reuse it.
		if p.status == "failed" && p.exitReasonCode == reason && fenceMatches {
			retryID, err := existingRetryID(ctx, tx, in.Fence.OrgID, in.Fence.RunID)
			if err != nil {
				return RecoverDispatchFailureOutcome{}, err
			}
			return RecoverDispatchFailureOutcome{Result: RecoverAlreadyRecovered, RetryRunID: retryID}, nil
		}
		// Terminal for another reason (or never ran): never overwrite history.
		return RecoverDispatchFailureOutcome{Result: RecoverParentNotRunning}, nil
	}
	if !fenceMatches {
		return RecoverDispatchFailureOutcome{Result: RecoverStaleAttempt}, nil
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE facebook_crawl_runs
		 SET status = 'failed', exit_reason_code = $3, finished_at = $4
		 WHERE org_id = $1 AND id = $2`,
		in.Fence.OrgID, in.Fence.RunID, reason, in.Now); err != nil {
		return RecoverDispatchFailureOutcome{}, err
	}

	retryID, err := insertOrReuseRetry(ctx, tx, in.Fence.OrgID, p, in.Fence.RunID)
	if err != nil {
		return RecoverDispatchFailureOutcome{}, err
	}
	return RecoverDispatchFailureOutcome{Result: RecoverRecovered, RetryRunID: retryID}, nil
}

// insertOrReuseRetry creates the one queued retry for a parent, keyed by
// ux_fb_crawl_runs_one_retry_per_parent so concurrent recoverers yield exactly
// one child; the loser re-reads it. account_id is NULL so the retry re-enters
// claim selection cleanly.
func insertOrReuseRetry(ctx context.Context, tx *sql.Tx, orgID int64, p parentRun, parentID int64) (int64, error) {
	var retryID int64
	err := tx.QueryRowContext(ctx,
		`INSERT INTO facebook_crawl_runs
		     (org_id, campaign_id, source_id, status, attempt, retry_of_run_id)
		 VALUES ($1, $2, $3, 'queued', $4, $5)
		 ON CONFLICT (org_id, retry_of_run_id) WHERE retry_of_run_id IS NOT NULL
		 DO NOTHING
		 RETURNING id`,
		orgID, p.campaignID, p.sourceID, p.attempt+1, parentID).Scan(&retryID)
	if err == nil {
		return retryID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return existingRetryID(ctx, tx, orgID, parentID)
}

func existingRetryID(ctx context.Context, tx *sql.Tx, orgID, parentID int64) (int64, error) {
	var retryID int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM facebook_crawl_runs WHERE org_id = $1 AND retry_of_run_id = $2`,
		orgID, parentID).Scan(&retryID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("crawlrun: parent recovered but retry row missing")
	}
	return retryID, err
}
