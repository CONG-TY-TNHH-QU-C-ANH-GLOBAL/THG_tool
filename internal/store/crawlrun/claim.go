package crawlrun

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// claimCandidateQuery locks one queued run whose campaign and source are both
// active and whose pool includes the requesting account. A sticky source
// (preferred_account_id set) is claimable only by that account; a null
// preference matches any pool account. FOR UPDATE OF r SKIP LOCKED lets
// concurrent claimers pass over each other's in-flight rows instead of blocking,
// so parallel schedulers never contend on the same run.
const claimCandidateQuery = `
SELECT r.id, r.campaign_id, r.source_id, r.attempt, c.freshness_window_minutes
FROM facebook_crawl_runs r
JOIN facebook_crawl_campaigns c
  ON c.org_id = r.org_id AND c.id = r.campaign_id
JOIN facebook_crawl_campaign_sources src
  ON src.org_id = r.org_id AND src.campaign_id = r.campaign_id AND src.id = r.source_id
JOIN facebook_crawl_campaign_accounts pa
  ON pa.org_id = r.org_id AND pa.campaign_id = r.campaign_id AND pa.account_id = $2
WHERE r.org_id = $1
  AND r.status = 'queued'
  AND c.status = 'active'
  AND src.status = 'active'
  AND (src.preferred_account_id IS NULL OR src.preferred_account_id = $2)
ORDER BY src.priority DESC, r.queued_at, r.id
FOR UPDATE OF r SKIP LOCKED
LIMIT 1`

// ClaimNextRun atomically transitions one eligible queued run to running for the
// given account, stamping the fresh cutoff derived from in.Now (the server
// clock; no database or Go local clock is read). The bool is false (nil error)
// only when no eligible run exists. Eligibility — active campaign and source,
// pool membership, and sticky-source affinity — is enforced in the candidate
// query; ux_fb_crawl_runs_one_active_account is the backstop if that account is
// already running elsewhere (that conflict surfaces as an error, never no-work).
func (s *Store) ClaimNextRun(ctx context.Context, in ClaimNextRunInput) (ClaimedRun, bool, error) {
	if err := s.requirePostgres(); err != nil {
		return ClaimedRun{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ClaimedRun{}, false, err
	}
	defer tx.Rollback()

	var (
		claimed       ClaimedRun
		windowMinutes int
	)
	err = tx.QueryRowContext(ctx, claimCandidateQuery, in.OrgID, in.AccountID).Scan(
		&claimed.RunID, &claimed.CampaignID, &claimed.SourceID, &claimed.Attempt, &windowMinutes)
	if errors.Is(err, sql.ErrNoRows) {
		return ClaimedRun{}, false, nil
	}
	if err != nil {
		return ClaimedRun{}, false, err
	}

	claimed.AccountID = in.AccountID
	claimed.FreshCutoffAt = in.Now.Add(-time.Duration(windowMinutes) * time.Minute)

	if _, err := tx.ExecContext(ctx,
		`UPDATE facebook_crawl_runs
		 SET status = 'running', account_id = $2, started_at = $3,
		     heartbeat_at = $3, fresh_cutoff_at = $4
		 WHERE org_id = $1 AND id = $5`,
		in.OrgID, in.AccountID, in.Now, claimed.FreshCutoffAt, claimed.RunID); err != nil {
		return ClaimedRun{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return ClaimedRun{}, false, err
	}
	return claimed, true, nil
}
