package outbound

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/store/dbutil"
)

// GetPolicy resolves the effective action policy for (orgID,
// actionType). Returns the org-specific row if present, else the
// global default (org_id=0). Returns sql.ErrNoRows if NEITHER exists
// — the caller must treat this as "action type not configured", a
// hard refusal.
func (s *Store) GetPolicy(ctx context.Context, orgID int64, actionType string) (*ActionPolicy, error) {
	return s.getPolicyDB(ctx, s.db, orgID, actionType)
}

// GetPolicyTx is the transactional twin so dedup checks running inside
// an enqueue tx read a snapshot consistent with their own writes.
func (s *Store) GetPolicyTx(ctx context.Context, tx *sql.Tx, orgID int64, actionType string) (*ActionPolicy, error) {
	return s.getPolicyDB(ctx, tx, orgID, actionType)
}

// dbQuerier abstracts *sql.DB and *sql.Tx so getPolicyDB serves both
// the non-tx and tx call paths without duplicating the SQL.
type dbQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func (s *Store) getPolicyDB(ctx context.Context, q dbQuerier, orgID int64, actionType string) (*ActionPolicy, error) {
	actionType = strings.TrimSpace(strings.ToLower(actionType))
	if actionType == "" {
		return nil, fmt.Errorf("outbound.policy: action_type required")
	}
	// One round-trip via ORDER BY org_id DESC LIMIT 1 — the org-specific
	// row (positive org_id) sorts above the global default (org_id=0)
	// when both exist, so the LIMIT 1 returns the right one without
	// needing a second query.
	const query = `
		SELECT id, org_id, action_type, dedup_scope,
		       block_on_planned, block_on_executing,
		       cooldown_seconds, conversation_aware,
		       created_at, COALESCE(updated_at, created_at)
		FROM action_policies
		WHERE action_type = ? AND (org_id = ? OR org_id = 0)
		ORDER BY org_id DESC
		LIMIT 1`
	var (
		p            ActionPolicy
		blockPlanned int
		blockExec    int
		convAware    int
		createdAtStr string
		updatedAtStr string
	)
	err := q.QueryRowContext(ctx, query, actionType, orgID).Scan(
		&p.ID, &p.OrgID, &p.ActionType, &p.DedupScope,
		&blockPlanned, &blockExec, &p.CooldownSeconds, &convAware,
		&createdAtStr, &updatedAtStr,
	)
	if err != nil {
		return nil, err
	}
	p.BlockOnPlanned = blockPlanned != 0
	p.BlockOnExecuting = blockExec != 0
	p.ConversationAware = convAware != 0
	p.CreatedAt = dbutil.ParseSQLiteTime(createdAtStr)
	p.UpdatedAt = dbutil.ParseSQLiteTime(updatedAtStr)
	return &p, nil
}

// UpsertPolicy lets an admin override the global default for an org.
// org_id must be > 0 — the global default row (org_id=0) is seeded at
// schema bootstrap and is not user-mutable.
func (s *Store) UpsertPolicy(ctx context.Context, p ActionPolicy) error {
	if p.OrgID <= 0 {
		return fmt.Errorf("outbound.policy: org_id must be > 0 (global defaults are read-only)")
	}
	if strings.TrimSpace(p.ActionType) == "" {
		return fmt.Errorf("outbound.policy: action_type required")
	}
	if !validDedupScope(p.DedupScope) {
		return fmt.Errorf("outbound.policy: dedup_scope %q invalid (want per_account|workspace|none)", p.DedupScope)
	}
	if p.CooldownSeconds < 0 {
		return fmt.Errorf("outbound.policy: cooldown_seconds must be >= 0")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO action_policies
		  (org_id, action_type, dedup_scope, block_on_planned, block_on_executing,
		   cooldown_seconds, conversation_aware, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(org_id, action_type) DO UPDATE SET
		  dedup_scope        = excluded.dedup_scope,
		  block_on_planned   = excluded.block_on_planned,
		  block_on_executing = excluded.block_on_executing,
		  cooldown_seconds   = excluded.cooldown_seconds,
		  conversation_aware = excluded.conversation_aware,
		  updated_at         = CURRENT_TIMESTAMP`,
		p.OrgID, strings.ToLower(p.ActionType), p.DedupScope,
		dbutil.BoolToInt(p.BlockOnPlanned), dbutil.BoolToInt(p.BlockOnExecuting),
		p.CooldownSeconds, dbutil.BoolToInt(p.ConversationAware),
	)
	return err
}

// ListPoliciesForOrg returns the policy set effective for an org:
// org-specific overrides where present, falling back to the global
// defaults. Used by the admin "coordination settings" dashboard.
func (s *Store) ListPoliciesForOrg(ctx context.Context, orgID int64) ([]ActionPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH effective AS (
		  SELECT action_type, MAX(org_id) AS picked_org_id
		    FROM action_policies
		    WHERE org_id = ? OR org_id = 0
		    GROUP BY action_type
		)
		SELECT p.id, p.org_id, p.action_type, p.dedup_scope,
		       p.block_on_planned, p.block_on_executing,
		       p.cooldown_seconds, p.conversation_aware,
		       p.created_at, COALESCE(p.updated_at, p.created_at)
		FROM action_policies p
		JOIN effective e ON e.action_type = p.action_type AND e.picked_org_id = p.org_id
		ORDER BY p.action_type`, orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActionPolicy
	for rows.Next() {
		var (
			p            ActionPolicy
			blockPlanned int
			blockExec    int
			convAware    int
			createdAtStr string
			updatedAtStr string
		)
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.ActionType, &p.DedupScope,
			&blockPlanned, &blockExec, &p.CooldownSeconds, &convAware,
			&createdAtStr, &updatedAtStr,
		); err != nil {
			return nil, err
		}
		p.BlockOnPlanned = blockPlanned != 0
		p.BlockOnExecuting = blockExec != 0
		p.ConversationAware = convAware != 0
		p.CreatedAt = dbutil.ParseSQLiteTime(createdAtStr)
		p.UpdatedAt = dbutil.ParseSQLiteTime(updatedAtStr)
		out = append(out, p)
	}
	return out, rows.Err()
}

func validDedupScope(s string) bool {
	switch s {
	case DedupScopePerAccount, DedupScopeWorkspace, DedupScopeNone:
		return true
	}
	return false
}
