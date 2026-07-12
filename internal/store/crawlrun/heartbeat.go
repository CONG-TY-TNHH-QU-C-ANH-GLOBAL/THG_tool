package crawlrun

import (
	"context"
	"time"
)

// Heartbeat records liveness for a running attempt. The fence match
// (org_id + id + attempt + status='running') makes a stale worker's heartbeat a
// zero-row no-op that can neither revive a terminal run nor touch a newer
// attempt. Returns true only when the running attempt was updated.
func (s *Store) Heartbeat(ctx context.Context, fence Fence, now time.Time) (bool, error) {
	if err := s.requirePostgres(); err != nil {
		return false, err
	}
	if !fence.valid() {
		return false, nil
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE facebook_crawl_runs SET heartbeat_at = $4
		 WHERE org_id = $1 AND id = $2 AND attempt = $3 AND status = 'running'`,
		fence.OrgID, fence.RunID, fence.Attempt, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
