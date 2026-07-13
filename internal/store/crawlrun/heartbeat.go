package crawlrun

import (
	"context"
	"fmt"
	"time"
)

// Heartbeat records liveness for a running attempt. The fence match
// (org_id + id + attempt + status='running') makes a stale worker's heartbeat a
// zero-row no-op that can neither revive a terminal run nor touch a newer
// attempt — every non-match collapses into HeartbeatStaleRejected rather than a
// distinguishable not-found. Exactly one updated row is HeartbeatUpdated.
func (s *Store) Heartbeat(ctx context.Context, fence Fence, now time.Time) (HeartbeatOutcome, error) {
	if err := s.requirePostgres(); err != nil {
		return "", err
	}
	if !fence.valid() {
		return HeartbeatStaleRejected, nil
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE facebook_crawl_runs SET heartbeat_at = $4
		 WHERE org_id = $1 AND id = $2 AND attempt = $3 AND status = 'running'`,
		fence.OrgID, fence.RunID, fence.Attempt, now)
	if err != nil {
		return "", fmt.Errorf("crawlrun: heartbeat exec: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("crawlrun: heartbeat rows affected: %w", err)
	}
	if n == 0 {
		return HeartbeatStaleRejected, nil
	}
	return HeartbeatUpdated, nil
}
