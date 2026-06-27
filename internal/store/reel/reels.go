package reel

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrReelNotFound is returned when a reel does not exist for the org (404 to the caller).
var ErrReelNotFound = errors.New("reel: not found")

// CreateReel inserts a draft reel and returns its id.
func (s *Store) CreateReel(r Reel) (int64, error) {
	if r.OrgID <= 0 {
		return 0, fmt.Errorf("reel.CreateReel: org_id required")
	}
	if r.Status == "" {
		r.Status = StatusDraft
	}
	if r.Source == "" {
		r.Source = "manual"
	}
	if r.TargetDurationSec <= 0 {
		r.TargetDurationSec = 25
	}
	if r.Keywords == "" {
		r.Keywords = "[]"
	}
	if r.ProductRefs == "" {
		r.ProductRefs = "[]"
	}
	res, err := s.db.Exec(
		`INSERT INTO reels (org_id, mission_id, created_by, source, status, brief_style, keywords, product_refs, target_duration_sec)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.OrgID, r.MissionID, r.CreatedBy, r.Source, r.Status, r.BriefStyle, r.Keywords, r.ProductRefs, r.TargetDurationSec,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetReel loads one reel scoped to the org. Returns ErrReelNotFound if absent.
func (s *Store) GetReel(orgID, id int64) (*Reel, error) {
	var r Reel
	var idem sql.NullString
	err := s.db.QueryRow(
		`SELECT id, org_id, mission_id, created_by, source, status, brief_style, keywords, product_refs,
		        target_duration_sec, COALESCE(render_idempotency_key, ''), final_output_key, total_cost_usd
		 FROM reels WHERE id = ? AND org_id = ?`, id, orgID,
	).Scan(&r.ID, &r.OrgID, &r.MissionID, &r.CreatedBy, &r.Source, &r.Status, &r.BriefStyle, &r.Keywords,
		&r.ProductRefs, &r.TargetDurationSec, &idem, &r.FinalOutputKey, &r.TotalCostUSD)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReelNotFound
	}
	if err != nil {
		return nil, err
	}
	r.RenderIdempotencyKey = idem.String
	return &r, nil
}

// ListReels returns an org's reels newest-first, capped at limit.
func (s *Store) ListReels(orgID int64, limit int) ([]Reel, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, org_id, mission_id, created_by, source, status, brief_style, keywords, product_refs,
		        target_duration_sec, COALESCE(render_idempotency_key, ''), final_output_key, total_cost_usd
		 FROM reels WHERE org_id = ? ORDER BY id DESC LIMIT ?`, orgID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reel
	for rows.Next() {
		var r Reel
		var idem sql.NullString
		if err := rows.Scan(&r.ID, &r.OrgID, &r.MissionID, &r.CreatedBy, &r.Source, &r.Status, &r.BriefStyle,
			&r.Keywords, &r.ProductRefs, &r.TargetDurationSec, &idem, &r.FinalOutputKey, &r.TotalCostUSD); err != nil {
			return nil, err
		}
		r.RenderIdempotencyKey = idem.String
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateReelStatus sets the lifecycle status (org-scoped).
func (s *Store) UpdateReelStatus(orgID, id int64, status string) error {
	_, err := s.db.Exec(
		`UPDATE reels SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND org_id = ?`,
		status, id, orgID,
	)
	return err
}

// SetFinalOutput records the assembled video key and flips the reel to its given status.
func (s *Store) SetFinalOutput(orgID, id int64, outputKey, status string) error {
	_, err := s.db.Exec(
		`UPDATE reels SET final_output_key = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND org_id = ?`,
		outputKey, status, id, orgID,
	)
	return err
}

// StartRenderCAS is the spend gate. It atomically sets the render_idempotency_key (which is
// UNIQUE) and moves the reel to rendering, but ONLY from an approvable state. Returns
// started=false (no error) when the reel is already rendering/terminal — the caller then
// reads current state and creates NO new render (idempotent approve). cost is never charged
// twice because shots are only created on a true start.
func (s *Store) StartRenderCAS(orgID, id int64, idempotencyKey string, leaseSeconds int) (started bool, err error) {
	res, err := s.db.Exec(
		`UPDATE reels
		   SET status = ?, render_idempotency_key = ?,
		       render_lease_expiry = DATETIME(CURRENT_TIMESTAMP, '+' || ? || ' seconds'),
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND org_id = ?
		   AND status IN (?, ?, ?)
		   AND render_idempotency_key IS NULL`,
		StatusRendering, idempotencyKey, leaseSeconds, id, orgID,
		StatusScriptReady, StatusApproved, StatusScripting,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// AddCost increments the accrued render cost (called once per shot completion).
func (s *Store) AddCost(orgID, id int64, deltaUSD float64) error {
	_, err := s.db.Exec(
		`UPDATE reels SET total_cost_usd = total_cost_usd + ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND org_id = ?`,
		deltaUSD, id, orgID,
	)
	return err
}
