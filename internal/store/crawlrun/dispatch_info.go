package crawlrun

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DispatchInfo is the source/campaign detail the PR-M4 orchestrator needs to
// build one crawl command for an already-claimed run: where to crawl and the
// campaign's per-run item cap. It carries no fence — the caller already owns the
// claimed run and pairs this with the run's fence when dispatching.
type DispatchInfo struct {
	SourceURL   string
	SourceLabel string
	MaxItems    int
}

// dispatchInfoQuery reads the source URL/label and the campaign item cap for one
// run's (campaign, source). Composite keys keep every row in the run's org.
const dispatchInfoQuery = `
SELECT s.source_url, s.source_label, c.max_items_per_run
FROM facebook_crawl_campaign_sources s
JOIN facebook_crawl_campaigns c
  ON c.org_id = s.org_id AND c.id = s.campaign_id
WHERE s.org_id = $1 AND s.campaign_id = $2 AND s.id = $3`

// DispatchInfo returns the crawl-target detail for a claimed run's source. It is
// a read, not a fenced mutation: the run is already 'running' and owned, so a
// concurrent source-URL edit at most redirects the next attempt. A missing row
// (source archived between claim and dispatch) is a real "gone" condition, not a
// stale fence, so sql.ErrNoRows propagates for the caller to recover the run.
func (s *Store) DispatchInfo(ctx context.Context, orgID, campaignID, sourceID int64) (DispatchInfo, error) {
	if err := s.requirePostgres(); err != nil {
		return DispatchInfo{}, err
	}
	var info DispatchInfo
	err := s.db.QueryRowContext(ctx, dispatchInfoQuery, orgID, campaignID, sourceID).
		Scan(&info.SourceURL, &info.SourceLabel, &info.MaxItems)
	if errors.Is(err, sql.ErrNoRows) {
		return DispatchInfo{}, err
	}
	if err != nil {
		return DispatchInfo{}, fmt.Errorf("crawlrun: dispatch info: %w", err)
	}
	return info, nil
}
