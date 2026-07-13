package crawlrun

import (
	"context"
	"fmt"
	"time"
)

// Heartbeat records liveness for a running attempt. A valid fence that matches
// no running row (wrong org/run/attempt, or a terminal status) collapses into
// HeartbeatStaleRejected — a stale-worker no-op that can neither revive a
// terminal run nor touch a newer attempt. Exactly one updated row is
// HeartbeatUpdated. An invalid fence is a caller error (ErrInvalidFence), not a
// stale state, so PR-M4 adapter/programming bugs cannot hide as stale rejects.
func (s *Store) Heartbeat(ctx context.Context, fence Fence, now time.Time) (HeartbeatOutcome, error) {
	if err := s.requirePostgres(); err != nil {
		return "", err
	}
	if !fence.valid() {
		return "", ErrInvalidFence
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
