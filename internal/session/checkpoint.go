package session

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// CheckpointManager handles the full lifecycle when Facebook presents a human-
// verification gate (checkpoint, CAPTCHA, identity confirmation).
//
// What it does:
//  1. Transitions the session active → checkpoint (no retry).
//  2. Persists the checkpoint URL so the UI can deep-link operators.
//  3. Fires an alert (Telegram / webhook) with the VNC URL.
//
// What it does NOT do:
//   - Retry the operation automatically.
//   - Attempt any automated CAPTCHA bypass (invariant NO_AUTOMATED_CAPTCHA_BYPASS).
//
// Operator resolves via VNC → calls ResolveCheckpoint() → session returns to ready.
type CheckpointManager struct {
	db      *sql.DB
	sm      *StateMachine
	alertFn func(msg string) // Telegram / webhook hook; nil = silent
}

func NewCheckpointManager(db *sql.DB, sm *StateMachine, alertFn func(string)) *CheckpointManager {
	return &CheckpointManager{db: db, sm: sm, alertFn: alertFn}
}

// Handle transitions the session to checkpoint state and notifies the operator.
// Returns nil — the caller must return status="human_required", not an error.
// Invariant: this function NEVER retries the failed operation.
func (m *CheckpointManager) Handle(
	ctx context.Context,
	accountID int64,
	accountName string,
	checkpointURL string,
	vncPort int,
	currentStatus Status,
) error {
	// Persist checkpoint URL before transition (audit trail).
	if _, err := m.db.ExecContext(ctx,
		`UPDATE browser_sessions SET checkpoint_url = ?, checkpoint_at = ? WHERE account_id = ?`,
		checkpointURL, time.Now(), accountID,
	); err != nil {
		slog.WarnContext(ctx, "checkpoint: failed to persist URL", "account_id", accountID, "error", err)
	}

	// State machine transition — active/idle → checkpoint.
	if err := m.sm.TransitionStatus(ctx, accountID, currentStatus, StatusCheckpoint,
		"checkpoint_manager", "facebook verification gate detected"); err != nil {
		slog.WarnContext(ctx, "checkpoint: state transition failed",
			"account_id", accountID, "from", currentStatus, "error", err)
		// Non-fatal: alert still fires even if DB transition fails.
	}

	// Increment account health counter.
	_, _ = m.db.ExecContext(ctx,
		`UPDATE accounts SET checkpoint_count = COALESCE(checkpoint_count, 0) + 1 WHERE id = ?`,
		accountID,
	)

	// Alert operator.
	if m.alertFn != nil {
		vncURL := fmt.Sprintf("https://sale.thgfulfill.com/api/browser/workspaces/%d/vnc", accountID)
		msg := fmt.Sprintf(
			"⚠️ CHECKPOINT — cần xử lý thủ công\n"+
				"Tài khoản: %s (ID %d)\n"+
				"URL checkpoint: %s\n"+
				"Mở VNC: %s\n"+
				"Sau khi xác minh xong → gọi /api/browser/workspaces/%d/resolve-checkpoint",
			accountName, accountID, checkpointURL, vncURL, accountID,
		)
		m.alertFn(msg)
	}

	slog.InfoContext(ctx, "checkpoint: session paused, human required",
		"account_id", accountID, "checkpoint_url", checkpointURL, "vnc_port", vncPort)
	return nil
}

// ResolveCheckpoint transitions the session from checkpoint → ready after an
// operator has manually passed the verification in the VNC viewer.
// Re-enabling the session allows the scheduler to assign new jobs to it.
func (m *CheckpointManager) ResolveCheckpoint(ctx context.Context, accountID int64) error {
	if err := m.sm.TransitionStatus(ctx, accountID,
		StatusCheckpoint, StatusReady,
		"operator", "human resolved checkpoint",
	); err != nil {
		return fmt.Errorf("resolve checkpoint: %w", err)
	}

	_, _ = m.db.ExecContext(ctx,
		`UPDATE browser_sessions SET checkpoint_url = '', checkpoint_at = NULL WHERE account_id = ?`,
		accountID,
	)

	slog.InfoContext(ctx, "checkpoint resolved — session ready", "account_id", accountID)
	return nil
}

// PendingCheckpoints returns all sessions currently awaiting human intervention.
// Used by GET /api/browser/checkpoints for the operator dashboard.
func (m *CheckpointManager) PendingCheckpoints(ctx context.Context) ([]PendingCheckpoint, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT bs.account_id, a.name, bs.checkpoint_url, bs.vnc_port, bs.checkpoint_at
		FROM browser_sessions bs
		JOIN accounts a ON a.id = bs.account_id
		WHERE bs.status = 'checkpoint'
		ORDER BY bs.checkpoint_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingCheckpoint
	for rows.Next() {
		var p PendingCheckpoint
		var at sql.NullTime
		if err := rows.Scan(&p.AccountID, &p.AccountName, &p.CheckpointURL, &p.VNCPort, &at); err != nil {
			continue
		}
		if at.Valid {
			p.DetectedAt = at.Time
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type PendingCheckpoint struct {
	AccountID     int64
	AccountName   string
	CheckpointURL string
	VNCPort       int
	DetectedAt    time.Time
}
