package postgres

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/models"
)

// ResetStaleExecutingForOrg returns abandoned executing rows to planned and
// clears their execution_id (so a stale in-flight report later fails its CAS).
// Two paths, matching internal/store/outbound.Store.ResetStaleExecuting:
// expired non-NULL lease, or legacy NULL-lease rows older than staleAfter.
// The best-effort per-row execution_attempts 'reset' audit append is NOT done
// here — that table is coordination-owned (see package doc); the UPDATE is the
// authoritative state change.
func (s *OutboundStore) ResetStaleExecutingForOrg(orgID int64, staleAfter time.Duration) error {
	if orgID <= 0 {
		return nil
	}
	if staleAfter <= 0 {
		staleAfter = 10 * time.Minute
	}

	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx,
		`UPDATE outbound_messages
		 SET execution_state = $1, verification_outcome = NULL,
		     claimed_by = '', claimed_at = NULL,
		     execution_id = '', lease_expiry = NULL
		 WHERE org_id = $2 AND execution_state = $3
		   AND (
		     (lease_expiry IS NOT NULL AND lease_expiry <= NOW())
		     OR (lease_expiry IS NULL AND claimed_at IS NOT NULL
		         AND claimed_at <= NOW() - make_interval(secs => $4))
		   )`,
		string(models.ExecPlanned), orgID, string(models.ExecExecuting),
		staleAfter.Seconds(),
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
