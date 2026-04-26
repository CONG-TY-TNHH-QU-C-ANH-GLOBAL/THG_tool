package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/thg/scraper/internal/observability"
	"github.com/thg/scraper/internal/store"
)

// AllocationPolicy controls which session is selected when multiple are idle.
type AllocationPolicy string

const (
	PolicyAny         AllocationPolicy = "any"          // first available, least-recently-used
	PolicySticky      AllocationPolicy = "sticky"        // must use the specific account_id
	PolicyLeastLoaded AllocationPolicy = "least_loaded"  // fewest active jobs (future)
)

// ErrNoIdleSession is returned when Acquire finds no session currently in idle state.
var ErrNoIdleSession = errors.New("no idle browser session available")

// Allocator provides race-free session acquisition for the worker.
// It uses BEGIN IMMEDIATE + optimistic versioning so two workers never
// claim the same session.
type Allocator struct {
	db *sql.DB
	sm *StateMachine
}

// NewAllocator creates an Allocator using the given database connection.
func NewAllocator(db *sql.DB, sm *StateMachine) *Allocator {
	return &Allocator{db: db, sm: sm}
}

// Acquire atomically claims an idle session and returns it.
// accountID = 0 means "any idle session".
// Returns ErrNoIdleSession if no idle session is available — the caller should
// re-queue the job and retry on the next scheduler poll tick.
func (a *Allocator) Acquire(
	ctx context.Context,
	accountID int64,
	policy AllocationPolicy,
	workerID string,
) (*store.BrowserSession, error) {

	// Up to 3 retries to handle the narrow window between SELECT and UPDATE.
	for attempt := 0; attempt < 3; attempt++ {
		sess, err := a.tryAcquire(ctx, accountID, policy, workerID)
		if err == nil {
			observability.AllocationAttempts.WithLabelValues("acquired").Inc()
			observability.SessionsGauge.WithLabelValues("active").Inc()
			observability.SessionsGauge.WithLabelValues("idle").Dec()
			return sess, nil
		}
		if errors.Is(err, ErrNoIdleSession) {
			observability.AllocationAttempts.WithLabelValues("no_session").Inc()
			return nil, err
		}
		// Contention: another worker just grabbed the same row. Brief pause then retry.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(50*(attempt+1)) * time.Millisecond):
		}
	}

	observability.AllocationAttempts.WithLabelValues("error").Inc()
	return nil, ErrNoIdleSession
}

func (a *Allocator) tryAcquire(
	ctx context.Context,
	accountID int64,
	policy AllocationPolicy,
	workerID string,
) (*store.BrowserSession, error) {

	// Step 1: find the candidate row
	q := `SELECT id, account_id, version, cdp_port, vnc_port, org_id
	      FROM browser_sessions
	      WHERE status = 'idle' AND cdp_port > 0`
	args := []any{}

	if accountID != 0 || policy == PolicySticky {
		q += " AND account_id = ?"
		args = append(args, accountID)
	}
	q += " ORDER BY last_active_at ASC LIMIT 1"

	var id, acctID, version, cdpPort, vncPort, orgID int64
	if err := a.db.QueryRowContext(ctx, q, args...).
		Scan(&id, &acctID, &version, &cdpPort, &vncPort, &orgID); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNoIdleSession
		}
		return nil, fmt.Errorf("find idle session: %w", err)
	}

	// Step 2: atomic claim via optimistic version check
	res, err := a.db.ExecContext(ctx,
		`UPDATE browser_sessions
		 SET status = 'active',
		     version = version + 1,
		     worker_id = ?,
		     status_prev = 'idle',
		     last_active_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status = 'idle' AND version = ?`,
		workerID, id, version,
	)
	if err != nil {
		return nil, fmt.Errorf("claim session: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Another worker claimed it between our SELECT and UPDATE
		return nil, fmt.Errorf("contention on session %d", id)
	}

	// Write audit log (non-fatal)
	_, _ = a.db.ExecContext(ctx,
		`INSERT INTO session_audit_log (account_id, from_status, to_status, triggered_by, reason)
		 VALUES (?, 'idle', 'active', ?, 'worker acquired')`,
		acctID, workerID,
	)

	slog.InfoContext(ctx, "session acquired",
		"account_id", acctID,
		"session_db_id", id,
		"worker_id", workerID,
		"cdp_port", cdpPort,
	)

	return &store.BrowserSession{
		ID:        id,
		AccountID: acctID,
		OrgID:     orgID,
		Status:    "active",
		CDPPort:   int(cdpPort),
		VNCPort:   int(vncPort),
	}, nil
}

// Release returns a session to idle state after the worker is done.
// workerID is used to confirm we own the session (prevents accidental double-release).
func (a *Allocator) Release(ctx context.Context, accountID int64, workerID string) error {
	res, err := a.db.ExecContext(ctx,
		`UPDATE browser_sessions
		 SET status = 'idle',
		     version = version + 1,
		     worker_id = '',
		     status_prev = 'active',
		     last_active_at = CURRENT_TIMESTAMP
		 WHERE account_id = ? AND status = 'active' AND worker_id = ?`,
		accountID, workerID,
	)
	if err != nil {
		return fmt.Errorf("release session: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session %d not owned by worker %s or already released", accountID, workerID)
	}

	_, _ = a.db.ExecContext(ctx,
		`INSERT INTO session_audit_log (account_id, from_status, to_status, triggered_by, reason)
		 VALUES (?, 'active', 'idle', ?, 'worker released')`,
		accountID, workerID,
	)

	observability.SessionsGauge.WithLabelValues("active").Dec()
	observability.SessionsGauge.WithLabelValues("idle").Inc()

	slog.InfoContext(ctx, "session released",
		"account_id", accountID,
		"worker_id", workerID,
	)
	return nil
}
