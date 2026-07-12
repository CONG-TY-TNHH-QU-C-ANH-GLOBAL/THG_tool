package crawlrun

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// dueSourcesQuery selects active sources of active campaigns whose cadence has
// elapsed and that have no open run yet. The NOT EXISTS only skips obviously-
// wasted inserts; the ux_fb_crawl_runs_one_open_source partial index is the real
// guard against a concurrent double-enqueue.
const dueSourcesQuery = `
SELECT s.id, s.campaign_id
FROM facebook_crawl_campaign_sources s
JOIN facebook_crawl_campaigns c
  ON c.org_id = s.org_id AND c.id = s.campaign_id
WHERE s.org_id = $1
  AND s.status = 'active'
  AND c.status = 'active'
  AND (s.last_run_at IS NULL
       OR s.last_run_at < $2 - make_interval(mins => c.cadence_minutes))
  AND NOT EXISTS (
      SELECT 1 FROM facebook_crawl_runs r
      WHERE r.org_id = s.org_id AND r.source_id = s.id
        AND r.status IN ('queued', 'waiting_for_connector_upgrade', 'running')
  )
ORDER BY s.priority DESC, s.id`

// EnqueueDueRuns inserts one queued run per due source. It creates no account
// lease and marks nothing running — claiming is a separate operation. Each
// per-source insert resolves its own open-source conflict idempotently, so the
// pass is safe to re-run and safe against a concurrent scheduler.
func (s *Store) EnqueueDueRuns(ctx context.Context, in EnqueueDueRunsInput) (EnqueueDueRunsOutcome, error) {
	var out EnqueueDueRunsOutcome
	if err := s.requirePostgres(); err != nil {
		return out, err
	}
	dues, err := s.dueSources(ctx, in)
	if err != nil {
		return out, err
	}
	for _, d := range dues {
		runID, reused, err := s.enqueueOne(ctx, in.OrgID, d.campaignID, d.sourceID)
		if err != nil {
			return out, err
		}
		if reused {
			out.ReusedRunIDs = append(out.ReusedRunIDs, runID)
		} else {
			out.CreatedRunIDs = append(out.CreatedRunIDs, runID)
		}
	}
	return out, nil
}

type dueSource struct {
	sourceID   int64
	campaignID int64
}

func (s *Store) dueSources(ctx context.Context, in EnqueueDueRunsInput) ([]dueSource, error) {
	rows, err := s.db.QueryContext(ctx, dueSourcesQuery, in.OrgID, in.Now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dues []dueSource
	for rows.Next() {
		var d dueSource
		if err := rows.Scan(&d.sourceID, &d.campaignID); err != nil {
			return nil, err
		}
		dues = append(dues, d)
	}
	return dues, rows.Err()
}

// enqueueOne inserts one queued run. Only the ux_fb_crawl_runs_one_open_source
// conflict means "already queued": on that exact constraint we re-read and reuse
// the existing open run; every other unique violation (active-account, retry-
// parent, task) and all non-unique failures propagate.
func (s *Store) enqueueOne(ctx context.Context, orgID, campaignID, sourceID int64) (int64, bool, error) {
	var runID int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO facebook_crawl_runs (org_id, campaign_id, source_id, status)
		 VALUES ($1, $2, $3, 'queued') RETURNING id`,
		orgID, campaignID, sourceID).Scan(&runID)
	if err == nil {
		return runID, false, nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "ux_fb_crawl_runs_one_open_source" {
		return 0, false, err
	}
	existing, err := s.openRunID(ctx, orgID, sourceID)
	if err != nil {
		return 0, false, err
	}
	return existing, true, nil
}

func (s *Store) openRunID(ctx context.Context, orgID, sourceID int64) (int64, error) {
	var runID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM facebook_crawl_runs
		 WHERE org_id = $1 AND source_id = $2
		   AND status IN ('queued', 'waiting_for_connector_upgrade', 'running')`,
		orgID, sourceID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		// The conflict said an open run exists but it vanished before re-read:
		// a concurrency/integrity fault, never a synthesized success.
		return 0, errors.New("crawlrun: open-source conflict but no open run found")
	}
	return runID, err
}
