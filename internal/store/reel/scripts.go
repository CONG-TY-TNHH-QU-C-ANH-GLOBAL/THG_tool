package reel

import (
	"context"
	"database/sql"
	"fmt"
)

// See the const block doc in reels.go: every statement here is a
// compile-time literal, Postgres-only, guarded by requirePostgres.
const (
	createScriptSQL = `INSERT INTO reel_scripts (org_id, reel_id, version, content) VALUES ($1, $2, $3, $4) RETURNING id`

	getLatestScriptSQL = `SELECT id, org_id, reel_id, version, content, approved, created_at FROM reel_scripts WHERE reel_id = $1 AND org_id = $2 ORDER BY version DESC LIMIT 1`

	listScriptsSQL = `SELECT id, org_id, reel_id, version, content, approved, created_at FROM reel_scripts WHERE reel_id = $1 AND org_id = $2 ORDER BY version ASC`

	approveScriptSQL = `UPDATE reel_scripts SET approved = 1 WHERE id = $1 AND org_id = $2`

	// approveScriptForReelSQL is approveScriptSQL plus a reel_id predicate, so
	// the atomic ApproveScriptAndSetReelStatus can only approve a script that
	// actually belongs to the reel whose status it is about to advance —
	// matching a script_id to the wrong reel_id is 0 rows, never a
	// cross-reel status change. Separate const so the older ApproveScript
	// (script_id + org_id only) keeps its shape.
	approveScriptForReelSQL = `UPDATE reel_scripts SET approved = 1 WHERE id = $1 AND org_id = $2 AND reel_id = $3`
)

func scanScript(row rowScanner) (*Script, error) {
	var sc Script
	var approved int
	if err := row.Scan(&sc.ID, &sc.OrgID, &sc.ReelID, &sc.Version, &sc.Content, &approved, &sc.CreatedAt); err != nil {
		return nil, err
	}
	sc.Approved = approved != 0
	return &sc, nil
}

// CreateScript inserts the next version of a reel's script draft. version
// must be the caller-computed next version number (UNIQUE(org_id, reel_id,
// version) rejects a duplicate). The composite FK (org_id, reel_id) ->
// reels(org_id, id) rejects a reel_id that does not belong to orgID.
func (s *Store) CreateScript(ctx context.Context, orgID, reelID int64, version int, content string) (int64, error) {
	if err := s.requirePostgres(); err != nil {
		return 0, err
	}
	if orgID <= 0 || reelID <= 0 {
		return 0, fmt.Errorf("reel: org_id and reel_id are required")
	}
	return s.insertReturningID(ctx, createScriptSQL, orgID, reelID, version, content)
}

// GetLatestScript returns the highest-version script draft for a reel, or
// sql.ErrNoRows if none exists or the reel belongs to a different org.
func (s *Store) GetLatestScript(ctx context.Context, orgID, reelID int64) (*Script, error) {
	if err := s.requirePostgres(); err != nil {
		return nil, err
	}
	if orgID <= 0 || reelID <= 0 {
		return nil, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx, getLatestScriptSQL, reelID, orgID)
	return scanScript(row)
}

// ListScripts returns every script version for a reel, oldest first.
func (s *Store) ListScripts(ctx context.Context, orgID, reelID int64) ([]*Script, error) {
	if err := s.requirePostgres(); err != nil {
		return nil, err
	}
	if orgID <= 0 || reelID <= 0 {
		return nil, fmt.Errorf("reel: org_id and reel_id are required")
	}
	rows, err := s.db.QueryContext(ctx, listScriptsSQL, reelID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Script
	for rows.Next() {
		sc, err := scanScript(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// ApproveScript marks a script version approved. Org-scoped: approving a
// script_id that belongs to a different org is a silent no-op.
func (s *Store) ApproveScript(ctx context.Context, orgID, scriptID int64) error {
	if err := s.requirePostgres(); err != nil {
		return err
	}
	if orgID <= 0 || scriptID <= 0 {
		return fmt.Errorf("reel: org_id and script_id are required")
	}
	_, err := s.db.ExecContext(ctx, approveScriptSQL, scriptID, orgID)
	return err
}
