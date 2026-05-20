package outbound

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// newExecutionID generates the per-claim idempotency token. 16
// crypto-random bytes hex-encoded — collision-free across realistic
// traffic and short enough to pass through HTTP bodies / Chrome
// message bus without padding overhead.
func newExecutionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read never fails on modern platforms, but if it
		// somehow does we still need an id. Fall back to a
		// time-derived value rather than crashing the claim path.
		return fmt.Sprintf("exec-fallback-%d", time.Now().UnixNano())
	}
	return "exec_" + hex.EncodeToString(b[:])
}

// Claim atomically moves one planned message into the executing
// state, stamps a fresh execution_id, and sets a lease_expiry so a
// stuck executor cannot pin the row beyond its budget. Returns the
// (execution_id, lease_expiry) the caller must thread out to the
// executor.
//
// CAS guarantees:
//
//   - execution_state must currently be "planned". Concurrent claimers
//     compete on this single atomic UPDATE; only one wins.
//   - org_id must match (cross-tenant defense).
//
// PR-2 (V2 staged refactor): the same tx ALSO appends a 'claim' row
// to execution_attempts as a best-effort audit trail. The ledger
// write is not load-bearing — a failed INSERT logs and continues;
// the CAS UPDATE on outbound_messages remains the authoritative state.
//
// Backward compatibility: leaseDuration == 0 falls back to
// [DefaultLease]. workerID is normalised to a default token when blank.
func (s *Store) Claim(orgID, id int64, workerID string, leaseDuration time.Duration) (*ClaimResult, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "chrome-extension"
	}
	if leaseDuration <= 0 {
		leaseDuration = DefaultLease
	}
	execID := newExecutionID()
	leaseExpiry := time.Now().UTC().Add(leaseDuration)

	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	// Row-level CAS — still the authoritative concurrency primitive.
	// PR-2 V2: no longer writes the legacy `status` column. Readers
	// consume execution_state directly.
	res, err := tx.ExecContext(ctx,
		`UPDATE outbound_messages
		 SET execution_state = ?,
		     claimed_by = ?, claimed_at = CURRENT_TIMESTAMP,
		     execution_id = ?, lease_expiry = ?
		 WHERE id = ? AND org_id = ? AND execution_state = ?`,
		models.ExecExecuting,
		workerID, execID, leaseExpiry,
		id, orgID, models.ExecPlanned,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, sql.ErrNoRows
	}

	// Look up action_type + account_id for the transition row (best-
	// effort; if the lookup fails we still write what we have).
	var actionType string
	var accountID int64
	var targetURL string
	_ = tx.QueryRowContext(ctx,
		`SELECT type, account_id, target_url FROM outbound_messages WHERE id = ? AND org_id = ?`,
		id, orgID,
	).Scan(&actionType, &accountID, &targetURL)

	s.appendTransition(ctx, tx, transitionInput{
		OutboundID:     id,
		OrgID:          orgID,
		AccountID:      accountID,
		TargetURL:      targetURL,
		ActionType:     actionType,
		Attempt:        s.nextAttemptNumber(ctx, tx, orgID, id),
		TransitionType: TransitionClaim,
		ExecutionID:    execID,
		ResultingState: models.ExecExecuting,
		LeaseExpiry:    sql.NullTime{Time: leaseExpiry, Valid: true},
	})

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ClaimResult{ExecutionID: execID, LeaseExpiry: leaseExpiry}, nil
}
