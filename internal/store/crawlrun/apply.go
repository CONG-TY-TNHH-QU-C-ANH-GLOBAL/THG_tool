package crawlrun

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ApplyRunResult applies one completed attempt's terminal result atomically:
// the run's terminal transition, its counters, the accepted fresh-lead identity
// reservations, and the source cursor/last-run stamp commit together or not at
// all. The run row is locked FOR UPDATE first, so concurrent applies for the
// same run serialize and exactly one can win; the loser observes the committed
// terminal row and classifies as exact replay or conflict. Campaign aggregates
// are not part of the approved runtime model and are deliberately not touched;
// canonical lead creation (lead_id back-fill) is deferred to the PR-M5 ingest
// consumer, so reservations commit with lead_id NULL.
func (s *Store) ApplyRunResult(ctx context.Context, in ApplyRunResultInput) (ApplyRunResultOutcome, error) {
	if err := s.requirePostgres(); err != nil {
		return ApplyRunResultOutcome{}, err
	}
	if err := in.validate(); err != nil {
		return ApplyRunResultOutcome{}, err
	}
	hashes, duplicates, err := normalizeCandidates(in.Leads)
	if err != nil {
		return ApplyRunResultOutcome{}, err
	}
	outcome := ApplyRunResultOutcome{
		CandidatesReceived: len(in.Leads),
		DuplicatesInBatch:  duplicates,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ApplyRunResultOutcome{}, fmt.Errorf("crawlrun: apply begin: %w", err)
	}
	defer tx.Rollback()

	run, err := lockRunForApply(ctx, tx, in.Fence.OrgID, in.Fence.RunID)
	if errors.Is(err, sql.ErrNoRows) {
		outcome.Result = ApplyStaleRejected
		return outcome, nil
	}
	if err != nil {
		return ApplyRunResultOutcome{}, err
	}

	outcome.Result = classifyApply(run, in)
	switch outcome.Result {
	case ApplyApplied:
		if err := applyLocked(ctx, tx, in, run, hashes, &outcome); err != nil {
			return ApplyRunResultOutcome{}, err
		}
		if err := tx.Commit(); err != nil {
			return ApplyRunResultOutcome{}, fmt.Errorf("crawlrun: apply commit: %w", err)
		}
	case ApplyAlreadyApplied:
		// Deterministic replay: report the committed reservation counts.
		if err := recountIndexed(ctx, tx, in.Fence, len(hashes), &outcome); err != nil {
			return ApplyRunResultOutcome{}, err
		}
	}
	return outcome, nil
}

// applyLocked performs the mutation phase under the held row lock: reserve lead
// identities, finish the run, stamp the source. Any error aborts the whole
// transaction via the caller's deferred rollback.
func applyLocked(ctx context.Context, tx *sql.Tx, in ApplyRunResultInput,
	run appliedRun, hashes []string, outcome *ApplyRunResultOutcome) error {
	indexed, err := reserveLeadIdentities(ctx, tx, in.Fence, hashes)
	if err != nil {
		return err
	}
	outcome.LeadsIndexed = indexed
	outcome.LeadsAlreadyKnown = len(hashes) - indexed
	if err := finishRun(ctx, tx, in); err != nil {
		return err
	}
	return stampSource(ctx, tx, in, run.sourceID)
}

// reserveLeadIdentities claims each identity via the pk_fb_crawl_lead_index
// primary key. A losing insert (identity already reserved by any run in this
// org) is the deterministic dedup outcome, not an error; every other constraint
// violation stays an error.
func reserveLeadIdentities(ctx context.Context, tx *sql.Tx, fence Fence, hashes []string) (int, error) {
	indexed := 0
	for _, hash := range hashes {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO facebook_crawl_lead_index (org_id, platform, post_dedup_hash, run_id)
			 VALUES ($1, 'facebook', $2, $3)
			 ON CONFLICT (org_id, platform, post_dedup_hash) DO NOTHING`,
			fence.OrgID, hash, fence.RunID)
		if err != nil {
			return 0, fmt.Errorf("crawlrun: reserve lead identity: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("crawlrun: reserve lead identity rows: %w", err)
		}
		indexed += int(n)
	}
	return indexed, nil
}

// finishRun writes the terminal transition under the full fence guard. The row
// is already locked and classified, so anything but exactly one updated row is
// an integrity violation that must abort the transaction.
func finishRun(ctx context.Context, tx *sql.Tx, in ApplyRunResultInput) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE facebook_crawl_runs
		 SET status = $4, exit_reason_code = $5, finished_at = $6,
		     posts_seen = $7, fresh_lead_count = $8, stale_skipped = $9,
		     duplicate_count = $10, unparsed_count = $11
		 WHERE org_id = $1 AND id = $2 AND attempt = $3 AND status = 'running'`,
		in.Fence.OrgID, in.Fence.RunID, in.Fence.Attempt,
		string(in.Status), in.ExitReasonCode, in.Now,
		in.Counters.PostsSeen, in.Counters.FreshLeadCount, in.Counters.StaleSkipped,
		in.Counters.DuplicateCount, in.Counters.UnparsedCount)
	if err != nil {
		return fmt.Errorf("crawlrun: finish run: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("crawlrun: finish run rows: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("crawlrun: finish run updated %d rows for a locked running attempt", n)
	}
	return nil
}

// stampSource records that the source completed a run (cadence input for the
// next enqueue pass) and advances the frontier cursor monotonically: GREATEST
// keeps an out-of-order replayed observation from moving the cursor backwards.
func stampSource(ctx context.Context, tx *sql.Tx, in ApplyRunResultInput, sourceID int64) error {
	var newest sql.NullTime
	if !in.NewestPostAt.IsZero() {
		newest = sql.NullTime{Time: in.NewestPostAt, Valid: true}
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE facebook_crawl_campaign_sources
		 SET last_run_at = $3,
		     cursor_last_post_at = CASE
		         WHEN $4::timestamptz IS NULL THEN cursor_last_post_at
		         ELSE GREATEST(COALESCE(cursor_last_post_at, $4), $4)
		     END,
		     updated_at = $3
		 WHERE org_id = $1 AND id = $2`,
		in.Fence.OrgID, sourceID, in.Now, newest)
	if err != nil {
		return fmt.Errorf("crawlrun: stamp source: %w", err)
	}
	return nil
}

// recountIndexed reports, for an exact replay, how many of the batch's
// identities this run owns in the committed index.
func recountIndexed(ctx context.Context, tx *sql.Tx, fence Fence, uniqueInBatch int,
	outcome *ApplyRunResultOutcome) error {
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM facebook_crawl_lead_index
		 WHERE org_id = $1 AND run_id = $2`,
		fence.OrgID, fence.RunID).Scan(&outcome.LeadsIndexed)
	if err != nil {
		return fmt.Errorf("crawlrun: recount indexed: %w", err)
	}
	outcome.LeadsAlreadyKnown = uniqueInBatch - outcome.LeadsIndexed
	return nil
}
