package reel

import (
	"database/sql"
	"errors"
)

// InsertScript appends a new script version for a reel. version should be the next
// integer (GetLatestScript().Version + 1, or 1 for the first). Returns the new id.
func (s *Store) InsertScript(sc Script) (int64, error) {
	if sc.Version <= 0 {
		sc.Version = 1
	}
	if sc.ShotList == "" {
		sc.ShotList = "[]"
	}
	if sc.VerifyFlags == "" {
		sc.VerifyFlags = "[]"
	}
	res, err := s.db.Exec(
		`INSERT INTO reel_scripts (reel_id, org_id, version, dialogue, shot_list, caption, verify_flags, approved)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sc.ReelID, sc.OrgID, sc.Version, sc.Dialogue, sc.ShotList, sc.Caption, sc.VerifyFlags, boolToInt(sc.Approved),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetLatestScript returns the highest-version script for a reel, or nil (no error)
// when the reel has no script yet.
func (s *Store) GetLatestScript(orgID, reelID int64) (*Script, error) {
	var sc Script
	var approved int
	err := s.db.QueryRow(
		`SELECT id, reel_id, org_id, version, dialogue, shot_list, caption, verify_flags, approved
		 FROM reel_scripts WHERE org_id = ? AND reel_id = ? ORDER BY version DESC LIMIT 1`, orgID, reelID,
	).Scan(&sc.ID, &sc.ReelID, &sc.OrgID, &sc.Version, &sc.Dialogue, &sc.ShotList, &sc.Caption, &sc.VerifyFlags, &approved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sc.Approved = approved != 0
	return &sc, nil
}

// ApproveLatestScript marks the latest script version approved (org-scoped).
func (s *Store) ApproveLatestScript(orgID, reelID int64) error {
	_, err := s.db.Exec(
		`UPDATE reel_scripts SET approved = 1
		 WHERE org_id = ? AND reel_id = ?
		   AND version = (SELECT MAX(version) FROM reel_scripts WHERE org_id = ? AND reel_id = ?)`,
		orgID, reelID, orgID, reelID,
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
