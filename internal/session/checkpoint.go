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
// CheckpointVerifier returns true when the live browser confirms the
// account is no longer at a Meta verification page. Implementations
// query CDP for document.URL / body text and look for the same patterns
// applied by applyFacebookHumanChallengeDetection.
//
// A nil verifier opts the workspace into the legacy "trust operator"
// flow — used in tests and for environments where CDP is not available.
type CheckpointVerifier interface {
	StillAtCheckpoint(ctx context.Context, accountID int64) (still bool, reason string, err error)
}

type CheckpointManager struct {
	db       *sql.DB
	sm       *StateMachine
	alertFn  func(msg string) // Telegram / webhook hook; nil = silent
	verifier CheckpointVerifier
}

func NewCheckpointManager(db *sql.DB, sm *StateMachine, alertFn func(string)) *CheckpointManager {
	return &CheckpointManager{db: db, sm: sm, alertFn: alertFn}
}

// SetVerifier attaches a browser-side probe that can confirm a checkpoint
// has actually been resolved before the state machine transitions back
// to ready. Without it, an operator could mark the session ready while
// Chrome is still parked on facebook.com/checkpoint, causing the very
// next worker pickup to bounce straight back into checkpoint state and
// thrash the restart loop.
func (m *CheckpointManager) SetVerifier(v CheckpointVerifier) {
	m.verifier = v
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

// ErrCheckpointStillActive is returned by ResolveCheckpoint when the
// browser still reports a verification page even though the operator
// asked to mark the session ready.
type ErrCheckpointStillActive struct {
	Reason string
}

func (e *ErrCheckpointStillActive) Error() string {
	if e.Reason == "" {
		return "checkpoint still active in browser"
	}
	return "checkpoint still active in browser: " + e.Reason
}

// ResolveCheckpoint transitions the session from checkpoint → ready after an
// operator has manually passed the verification in the VNC viewer.
// Re-enabling the session allows the scheduler to assign new jobs to it.
//
// When a verifier is wired (production), the live CDP page is checked
// before the transition. If the URL / body still matches a Facebook
// verification gate, the call returns *ErrCheckpointStillActive without
// touching the state machine. Handlers should map this to HTTP 409 so
// the operator knows to actually finish the verification.
func (m *CheckpointManager) ResolveCheckpoint(ctx context.Context, accountID int64) error {
	if m.verifier != nil {
		still, reason, err := m.verifier.StillAtCheckpoint(ctx, accountID)
		if err != nil {
			slog.WarnContext(ctx, "checkpoint verifier error — falling back to operator trust",
				"account_id", accountID, "error", err)
		} else if still {
			slog.WarnContext(ctx, "checkpoint resolve refused — browser still on verification page",
				"account_id", accountID, "reason", reason)
			return &ErrCheckpointStillActive{Reason: reason}
		}
	}

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
