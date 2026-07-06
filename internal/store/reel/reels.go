package reel

import (
	"context"
	"database/sql"
	"fmt"
)

const reelSelect = `SELECT id, org_id, title, brief, status, created_by, created_at, updated_at FROM reels`

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
	if orgID <= 0 {
		return 0, fmt.Errorf("reel: org_id is required")
	}
	return s.insertReturningID(ctx,
		`INSERT INTO reels (org_id, title, brief, status, created_by) VALUES (?, ?, ?, 'draft', ?) RETURNING id`,
		orgID, title, brief, createdBy,
	)
}

// GetReel returns the reel owned by orgID, or sql.ErrNoRows if no such row
// exists OR the row belongs to a different org — both cases look identical
// to the caller, matching the convention in internal/store/knowledge.
func (s *Store) GetReel(ctx context.Context, orgID, reelID int64) (*Reel, error) {
	if orgID <= 0 || reelID <= 0 {
		return nil, sql.ErrNoRows
	}
	row := s.queryRowContext(ctx, reelSelect+` WHERE id = ? AND org_id = ?`, reelID, orgID)
	return scanReel(row)
}

// ListReels returns every reel owned by orgID, newest first.
func (s *Store) ListReels(ctx context.Context, orgID int64) ([]*Reel, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("reel: org_id is required")
	}
	rows, err := s.queryContext(ctx, reelSelect+` WHERE org_id = ? ORDER BY created_at DESC`, orgID)
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
	if orgID <= 0 || reelID <= 0 {
		return fmt.Errorf("reel: org_id and reel_id are required")
	}
	_, err := s.execContext(ctx,
		`UPDATE reels SET status = ?, updated_at = `+s.dialect.NowExpr()+` WHERE id = ? AND org_id = ?`,
		status, reelID, orgID,
	)
	return err
}
