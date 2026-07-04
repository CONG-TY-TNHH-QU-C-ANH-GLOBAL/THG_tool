// Domain: coordination (see internal/store/DOMAINS.md)
package coordination

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
)

// ActionLedgerEntry records one outbound action attempted by one account on
// one target. It is the foundation table of the Coordination Plane: the
// future orchestrator + behaviour-profile PRs consult this to decide spacing,
// account rotation, and rate caps. PR-1 only writes; richer policy reads are
// future work. See project_distributed_coordination.md.
type ActionLedgerEntry struct {
	ID            int64
	OrgID         int64
	ActionType    string // comment | inbox | group_post | profile_post | ...
	TargetType    string // post | profile | group (derived from ActionType)
	TargetURL     string
	AccountID     int64
	CreatedBy     int64 // member who initiated the action (immutable execution ownership); 0 = system
	OutboundID    int64 // FK to outbound_messages.id; 0 when unattached
	PerformedAt   time.Time
	CooldownUntil time.Time
	Outcome       string // queued | succeeded | failed | skipped
	Reason        string
}

// LedgerOutcome enumerates the values the outcome column may take.
const (
	LedgerOutcomeQueued    = "queued"
	LedgerOutcomeSucceeded = "succeeded"
	LedgerOutcomeFailed    = "failed"
	LedgerOutcomeSkipped   = "skipped"
)

// targetTypeFromAction maps an action_type to its conceptual target. Kept as a
// small lookup so callers do not duplicate the mapping. Unknown action types
// return "" — the column is non-NULL but a blank value is acceptable.
func targetTypeFromAction(actionType string) string {
	switch strings.ToLower(strings.TrimSpace(actionType)) {
	case "comment":
		return "post"
	case "inbox":
		return "profile"
	case "group_post":
		return "group"
	case "profile_post":
		return "profile"
	default:
		return ""
	}
}

// RecordLedgerTx writes a ledger row inside an open transaction. Used by
// outbound's queue path via the Hooks closure (see
// `installOutboundHooks` in `internal/store/outbound_hooks.go`) so the
// outbound row and its ledger entry land in the same transaction. Cooldown
// <= 0 leaves cooldown_until NULL (planner uses defaults).
//
// Phase 5B: exported (was `recordActionLedgerTx`) because the hooks
// closure now lives across the package boundary. Package-level function
// — no Store state required, the caller threads its own tx.
func RecordLedgerTx(tx *sql.Tx, orgID, accountID, createdBy int64, actionType, targetURL string, outboundID int64, cooldown time.Duration) error {
	if orgID <= 0 || strings.TrimSpace(actionType) == "" || strings.TrimSpace(targetURL) == "" {
		return fmt.Errorf("ledger requires org_id, action_type, target_url")
	}
	var cooldownUntil any
	if cooldown > 0 {
		cooldownUntil = time.Now().UTC().Add(cooldown).Format("2006-01-02 15:04:05")
	}
	_, err := tx.Exec(
		`INSERT INTO action_ledger
			(org_id, action_type, target_type, target_url, account_id, created_by, outbound_id,
			 performed_at, cooldown_until, outcome, reason)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?, '')`,
		orgID, actionType, targetTypeFromAction(actionType), targetURL,
		accountID, createdBy, outboundID, cooldownUntil, LedgerOutcomeQueued,
	)
	return err
}

// RecordActionLedger inserts a standalone ledger row outside any outbound
// transaction. Use when an action happens that is not driven by
// QueueOutboundForOrg (e.g. a manual record or a future external action).
func (s *Store) RecordActionLedger(ctx context.Context, entry ActionLedgerEntry) (int64, error) {
	if entry.OrgID <= 0 || strings.TrimSpace(entry.ActionType) == "" || strings.TrimSpace(entry.TargetURL) == "" {
		return 0, fmt.Errorf("ledger requires org_id, action_type, target_url")
	}
	if entry.TargetType == "" {
		entry.TargetType = targetTypeFromAction(entry.ActionType)
	}
	if entry.Outcome == "" {
		entry.Outcome = LedgerOutcomeQueued
	}
	var performedAt any
	if entry.PerformedAt.IsZero() {
		performedAt = nil
	} else {
		performedAt = entry.PerformedAt.UTC().Format("2006-01-02 15:04:05")
	}
	var cooldownUntil any
	if !entry.CooldownUntil.IsZero() {
		cooldownUntil = entry.CooldownUntil.UTC().Format("2006-01-02 15:04:05")
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO action_ledger
			(org_id, action_type, target_type, target_url, account_id, created_by, outbound_id,
			 performed_at, cooldown_until, outcome, reason)
		 VALUES (?, ?, ?, ?, ?, ?, ?, COALESCE(?, CURRENT_TIMESTAMP), ?, ?, ?)`,
		entry.OrgID, entry.ActionType, entry.TargetType, entry.TargetURL,
		entry.AccountID, entry.CreatedBy, entry.OutboundID, performedAt, cooldownUntil, entry.Outcome, entry.Reason,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListActionLedger returns recent ledger entries for an org, optionally
// filtered by action_type and target_url, ordered most-recent first. When
// since is non-zero, only entries newer than since are returned. limit
// defaults to 100 (max 500). This is the canonical read API for the future
// orchestrator + dashboards.
func (s *Store) ListActionLedger(ctx context.Context, orgID int64, actionType, targetURL string, since time.Time, limit int) ([]ActionLedgerEntry, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("org_id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	query := `SELECT id, org_id, action_type, COALESCE(target_type,''), target_url, account_id,
		         outbound_id, performed_at, COALESCE(cooldown_until,''), outcome, COALESCE(reason,'')
		 FROM action_ledger
		 WHERE org_id = ?`
	args := []any{orgID}
	if t := strings.TrimSpace(actionType); t != "" {
		query += ` AND action_type = ?`
		args = append(args, t)
	}
	if u := strings.TrimSpace(targetURL); u != "" {
		query += ` AND target_url = ?`
		args = append(args, u)
	}
	if !since.IsZero() {
		query += ` AND performed_at >= ?`
		args = append(args, since.UTC().Format("2006-01-02 15:04:05"))
	}
	query += ` ORDER BY performed_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ActionLedgerEntry
	for rows.Next() {
		var e ActionLedgerEntry
		var performedAt, cooldownUntil string
		if err := rows.Scan(&e.ID, &e.OrgID, &e.ActionType, &e.TargetType, &e.TargetURL,
			&e.AccountID, &e.OutboundID, &performedAt, &cooldownUntil, &e.Outcome, &e.Reason); err != nil {
			return nil, err
		}
		e.PerformedAt = dbutil.ParseSQLiteTime(performedAt)
		if cooldownUntil != "" {
			e.CooldownUntil = dbutil.ParseSQLiteTime(cooldownUntil)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkActionLedgerOutcomeByOutbound updates the ledger entry tied to a given
// outbound_messages.id. The Execution Verification layer (Step 3) uses this
// as its single write-point: the verifier classifies an outcome and propagates
// it to the ledger by outbound id without needing to know the ledger id
// separately. Returns (ledgerID, error) so the caller can record the linkage
// on the execution_attempts row. ledgerID=0 when no matching ledger row
// exists (rare — possible for manually-sent outbounds that bypassed the queue).
func (s *Store) MarkActionLedgerOutcomeByOutbound(ctx context.Context, orgID, outboundID int64, outcome, reason string) (int64, error) {
	if orgID <= 0 || outboundID <= 0 {
		return 0, fmt.Errorf("org_id and outbound_id are required")
	}
	outcome = strings.TrimSpace(outcome)
	if outcome == "" {
		outcome = LedgerOutcomeQueued
	}
	var ledgerID int64
	row := s.db.QueryRowContext(ctx,
		`SELECT id FROM action_ledger WHERE org_id = ? AND outbound_id = ?
		  ORDER BY performed_at DESC LIMIT 1`,
		orgID, outboundID,
	)
	if err := row.Scan(&ledgerID); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE action_ledger SET outcome = ?, reason = ? WHERE id = ?`,
		outcome, reason, ledgerID,
	); err != nil {
		return ledgerID, err
	}
	return ledgerID, nil
}
