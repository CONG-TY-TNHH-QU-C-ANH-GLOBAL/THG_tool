package reel

import (
	"context"
	"database/sql"
	"fmt"
)

// Every SQL statement below is a compile-time const literal — never built
// via fmt.Sprintf, string concatenation, or a dialect-selection function
// call. Table/column names and status literals are fixed in source;
// org_id, reel_id, title, brief, status, and created_by travel only as
// bound $N parameters, never interpolated. Postgres-only: requirePostgres
// (called first by every method below) guarantees no other dialect ever
// reaches these queries.
const (
	createReelSQL = `INSERT INTO reels (org_id, title, brief, status, created_by) VALUES ($1, $2, $3, 'draft', $4) RETURNING id`

	getReelSQL = `SELECT id, org_id, title, brief, status, created_by, created_at, updated_at FROM reels WHERE id = $1 AND org_id = $2`

	listReelsSQL = `SELECT id, org_id, title, brief, status, created_by, created_at, updated_at FROM reels WHERE org_id = $1 ORDER BY created_at DESC`

	updateReelStatusSQL = `UPDATE reels SET status = $1, updated_at = NOW() WHERE id = $2 AND org_id = $3`
)

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanReel(row rowScanner) (*Reel, error) {
	var r Reel
	if err := row.Scan(&r.ID, &r.OrgID, &r.Title, &r.Brief, &r.Status, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateReel inserts a new reel in 'draft' status and returns its id.
func (s *Store) CreateReel(ctx context.Context, orgID int64, title, brief string, createdBy int64) (int64, error) {
	if err := s.requirePostgres(); err != nil {
		return 0, err
	}
	if orgID <= 0 {
		return 0, fmt.Errorf("reel: org_id is required")
	}
	return s.insertReturningID(ctx, createReelSQL, orgID, title, brief, createdBy)
}

// GetReel returns the reel owned by orgID, or sql.ErrNoRows if no such row
// exists OR the row belongs to a different org — both cases look identical
// to the caller, matching the convention in internal/store/knowledge.
func (s *Store) GetReel(ctx context.Context, orgID, reelID int64) (*Reel, error) {
	if err := s.requirePostgres(); err != nil {
		return nil, err
	}
	if orgID <= 0 || reelID <= 0 {
		return nil, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx, getReelSQL, reelID, orgID)
	return scanReel(row)
}

// ListReels returns every reel owned by orgID, newest first.
func (s *Store) ListReels(ctx context.Context, orgID int64) ([]*Reel, error) {
	if err := s.requirePostgres(); err != nil {
		return nil, err
	}
	if orgID <= 0 {
		return nil, fmt.Errorf("reel: org_id is required")
	}
	rows, err := s.db.QueryContext(ctx, listReelsSQL, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Reel
	for rows.Next() {
		r, err := scanReel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateReelStatus sets a reel's lifecycle status. Org-scoped: updating a
// reel_id that belongs to a different org is a silent no-op (0 rows
// affected), never a cross-tenant mutation.
func (s *Store) UpdateReelStatus(ctx context.Context, orgID, reelID int64, status string) error {
	if err := s.requirePostgres(); err != nil {
		return err
	}
	if orgID <= 0 || reelID <= 0 {
		return fmt.Errorf("reel: org_id and reel_id are required")
	}
	_, err := s.db.ExecContext(ctx, updateReelStatusSQL, status, reelID, orgID)
	return err
}
