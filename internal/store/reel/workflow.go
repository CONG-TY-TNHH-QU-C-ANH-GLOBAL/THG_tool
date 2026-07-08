package reel

import (
	"context"
	"fmt"
)

// Each reel workflow step is two row writes that must land together: a
// reel_scripts write plus the reel's own status UPDATE. These methods wrap
// each pair in ONE Postgres transaction so a mid-step failure can never
// leave the script changed while reels.status lags (or vice versa) — the
// partial state CodeRabbit flagged on PR-R2 before any public API existed.
//
// Domain-owned transaction, same pattern as
// internal/store/knowledge/sources.go DeleteSourceForOrg: the store owns its
// own tx, no parent-threaded *sql.Tx, no generic unit-of-work. Every
// statement is one of the Postgres-only const literals in reels.go/scripts.go
// (RETURNING/UPDATE), guarded by requirePostgres — see those files' doc.

// CreateScriptAndSetReelStatus inserts one script version and moves the reel
// to status in a single transaction, returning the new script id. version is
// the caller-computed next version: UNIQUE(org_id, reel_id, version) still
// rejects a duplicate (the version-race loser errors and the whole tx rolls
// back — no status change leaks), and the composite FK still rejects a
// cross-org reel_id.
func (s *Store) CreateScriptAndSetReelStatus(ctx context.Context, orgID, reelID int64, version int, content, status string) (scriptID int64, err error) {
	if err := s.requirePostgres(); err != nil {
		return 0, err
	}
	if orgID <= 0 || reelID <= 0 {
		return 0, fmt.Errorf("reel: org_id and reel_id are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = tx.QueryRowContext(ctx, createScriptSQL, orgID, reelID, version, content).Scan(&scriptID); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, updateReelStatusSQL, status, reelID, orgID); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return scriptID, nil
}

// ApproveScriptAndSetReelStatus marks scriptID approved and moves the reel to
// status in a single transaction. Org-scoped: a scriptID/reelID owned by a
// different org matches zero rows in both statements (no cross-tenant write),
// and approval — which gates RenderFake — commits atomically with the reel's
// status so a failure never leaves a script approved while the reel looks
// un-approved.
func (s *Store) ApproveScriptAndSetReelStatus(ctx context.Context, orgID, reelID, scriptID int64, status string) (err error) {
	if err := s.requirePostgres(); err != nil {
		return err
	}
	if orgID <= 0 || reelID <= 0 || scriptID <= 0 {
		return fmt.Errorf("reel: org_id, reel_id and script_id are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, approveScriptSQL, scriptID, orgID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, updateReelStatusSQL, status, reelID, orgID); err != nil {
		return err
	}
	return tx.Commit()
}
