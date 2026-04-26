package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
)

// Status represents a browser session lifecycle state.
type Status string

const (
	StatusInitializing Status = "initializing"
	StatusReady        Status = "ready"
	StatusActive       Status = "active"
	StatusIdle         Status = "idle"
	StatusRecovering   Status = "recovering"
	StatusTerminated   Status = "terminated"
)

var allowedTransitions = map[[2]Status]bool{
	{StatusInitializing, StatusReady}:      true,
	{StatusReady, StatusActive}:            true,
	{StatusActive, StatusIdle}:             true,
	{StatusIdle, StatusActive}:             true,
	{StatusIdle, StatusRecovering}:         true,
	{StatusActive, StatusRecovering}:       true,
	{StatusRecovering, StatusReady}:        true,
	{StatusRecovering, StatusTerminated}:   true,
	{StatusReady, StatusTerminated}:        true,
	{StatusIdle, StatusTerminated}:         true,
	{StatusActive, StatusTerminated}:       true,
}

// ErrInvalidTransition is returned when a state change is not in allowedTransitions.
var ErrInvalidTransition = errors.New("invalid session state transition")

// ErrStaleTransition is returned when the session's current status no longer
// matches `from` — another caller already changed it.
var ErrStaleTransition = errors.New("stale session state transition")

// StateMachine enforces lifecycle rules for browser sessions.
// Every status write in the system must go through TransitionStatus.
type StateMachine struct {
	db *sql.DB
}

// NewStateMachine creates a StateMachine backed by the given database connection.
func NewStateMachine(db *sql.DB) *StateMachine {
	return &StateMachine{db: db}
}

// TransitionStatus atomically validates and applies a status change for accountID.
// It also writes an audit row to session_audit_log.
//
//   - Returns ErrInvalidTransition if [from→to] is not in the allowed set.
//   - Returns ErrStaleTransition if the row's current status differs from `from`
//     (another goroutine already changed it).
func (sm *StateMachine) TransitionStatus(
	ctx context.Context,
	accountID int64,
	from, to Status,
	triggeredBy, reason string,
) error {
	if !allowedTransitions[[2]Status{from, to}] {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
	}

	tx, err := sm.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx,
		`UPDATE browser_sessions
		 SET status = ?, status_prev = ?, version = version + 1
		 WHERE account_id = ? AND status = ?`,
		string(to), string(from), accountID, string(from),
	)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: account %d expected status=%s", ErrStaleTransition, accountID, from)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO session_audit_log (account_id, from_status, to_status, triggered_by, reason)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, string(from), string(to), triggeredBy, reason,
	)
	if err != nil {
		return fmt.Errorf("audit log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	slog.InfoContext(ctx, "session status transition",
		"account_id", accountID,
		"from", string(from),
		"to", string(to),
		"triggered_by", triggeredBy,
	)
	return nil
}

// ForceTerminate sets status to 'terminated' regardless of current state.
// Used only by the circuit breaker and restart controller after max retries.
func (sm *StateMachine) ForceTerminate(ctx context.Context, accountID int64, reason string) error {
	_, err := sm.db.ExecContext(ctx,
		`UPDATE browser_sessions
		 SET status = 'terminated', status_prev = status, version = version + 1
		 WHERE account_id = ? AND status != 'terminated'`,
		accountID,
	)
	if err != nil {
		return err
	}
	_, _ = sm.db.ExecContext(ctx,
		`INSERT INTO session_audit_log (account_id, from_status, to_status, triggered_by, reason)
		 VALUES (?, (SELECT status_prev FROM browser_sessions WHERE account_id=?), 'terminated', 'force', ?)`,
		accountID, accountID, reason,
	)
	return nil
}

// CurrentStatus returns the current status of the session for accountID.
// Returns empty string if no session exists.
func (sm *StateMachine) CurrentStatus(ctx context.Context, accountID int64) (Status, error) {
	var s string
	err := sm.db.QueryRowContext(ctx,
		`SELECT status FROM browser_sessions WHERE account_id=?`, accountID,
	).Scan(&s)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return Status(s), err
}
