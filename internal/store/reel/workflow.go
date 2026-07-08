package reel

import (
	"context"
	"database/sql"
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

// withTx runs fn inside a single transaction, committing on success and
// rolling back if fn returns an error. Local convenience for this package's
// two-write workflow methods only — deliberately not a UnitOfWork/TxManager,
// no parent-threaded *sql.Tx.
func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

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
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, createScriptSQL, orgID, reelID, version, content).Scan(&scriptID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, updateReelStatusSQL, status, reelID, orgID)
		return err
	})
	if err != nil {
		return 0, err
	}
	return scriptID, nil
}

// ApproveScriptAndSetReelStatus marks scriptID approved and moves reelID to
// status in a single transaction. The approval is scoped by org_id AND
// reel_id, so a scriptID that does not belong to reelID (or to orgID) matches
// zero rows: the method returns sql.ErrNoRows and the reel status is NOT
// touched — a mismatched pairing can never approve one reel's script while
// advancing a different reel's status. Approval gates RenderFake, so it
// commits atomically with the reel status; any failure rolls both back.
func (s *Store) ApproveScriptAndSetReelStatus(ctx context.Context, orgID, reelID, scriptID int64, status string) error {
	if err := s.requirePostgres(); err != nil {
		return err
	}
	if orgID <= 0 || reelID <= 0 || scriptID <= 0 {
		return fmt.Errorf("reel: org_id, reel_id and script_id are required")
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, approveScriptForReelSQL, scriptID, orgID, reelID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return sql.ErrNoRows
		}
		_, err = tx.ExecContext(ctx, updateReelStatusSQL, status, reelID, orgID)
		return err
	})
}
