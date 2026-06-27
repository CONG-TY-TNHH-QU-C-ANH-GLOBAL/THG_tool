package reel

import "fmt"

// CreateShot inserts one planned shot. UNIQUE(reel_id, scene) makes this idempotent
// across retries of the same approve (a second insert of the same scene is a no-op error
// the caller can treat as already-created).
func (s *Store) CreateShot(sh Shot) error {
	if sh.OrgID <= 0 || sh.ReelID <= 0 {
		return fmt.Errorf("reel.CreateShot: org_id and reel_id required")
	}
	if sh.RenderState == "" {
		sh.RenderState = ShotPlanned
	}
	if sh.Kind == "" {
		sh.Kind = "broll"
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO reel_shots (reel_id, org_id, scene, kind, render_state)
		 VALUES (?, ?, ?, ?, ?)`,
		sh.ReelID, sh.OrgID, sh.Scene, sh.Kind, sh.RenderState,
	)
	return err
}

// ClaimShotForRender CASes a single shot planned → rendering, stamping the provider job id
// and a lease. This is the per-shot spend commitment. Returns claimed=false (no error) if
// the shot is not in planned (already rendering/done) — idempotent under retry.
func (s *Store) ClaimShotForRender(orgID, reelID, scene int64, provider, providerJobID string, leaseSeconds int) (claimed bool, err error) {
	res, err := s.db.Exec(
		`UPDATE reel_shots
		   SET render_state = ?, provider = ?, provider_job_id = ?, attempts = attempts + 1,
		       lease_expiry = DATETIME(CURRENT_TIMESTAMP, '+' || ? || ' seconds')
		 WHERE org_id = ? AND reel_id = ? AND scene = ? AND render_state = ?`,
		ShotRendering, provider, providerJobID, leaseSeconds, orgID, reelID, scene, ShotPlanned,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// MarkShotDone CASes a shot rendering → done, idempotent by provider_job_id: a redelivered
// webhook for an already-done shot affects 0 rows, so the caller must NOT add cost again.
// Returns applied=true only on the first-win transition.
func (s *Store) MarkShotDone(orgID, reelID int64, providerJobID, outputKey string, costUSD float64) (applied bool, err error) {
	res, err := s.db.Exec(
		`UPDATE reel_shots
		   SET render_state = ?, output_key = ?, cost_usd = ?, lease_expiry = NULL
		 WHERE org_id = ? AND reel_id = ? AND provider_job_id = ? AND render_state = ?`,
		ShotDone, outputKey, costUSD, orgID, reelID, providerJobID, ShotRendering,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// MarkShotFailed records a provider hard-failure (no spend). The shot can be retried
// individually later; the reel is not auto re-rendered.
func (s *Store) MarkShotFailed(orgID, reelID int64, providerJobID string) error {
	_, err := s.db.Exec(
		`UPDATE reel_shots SET render_state = ?, lease_expiry = NULL
		 WHERE org_id = ? AND reel_id = ? AND provider_job_id = ? AND render_state = ?`,
		ShotFailed, orgID, reelID, providerJobID, ShotRendering,
	)
	return err
}

// CountShots returns how many shots exist and how many are done for a reel.
func (s *Store) CountShots(orgID, reelID int64) (total, done int, err error) {
	err = s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN render_state = ? THEN 1 ELSE 0 END), 0)
		 FROM reel_shots WHERE org_id = ? AND reel_id = ?`,
		ShotDone, orgID, reelID,
	).Scan(&total, &done)
	return total, done, err
}

// ListShots returns a reel's shots in scene order.
func (s *Store) ListShots(orgID, reelID int64) ([]Shot, error) {
	rows, err := s.db.Query(
		`SELECT id, reel_id, org_id, scene, kind, render_state, provider, provider_job_id, output_key, cost_usd, attempts
		 FROM reel_shots WHERE org_id = ? AND reel_id = ? ORDER BY scene ASC`, orgID, reelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Shot
	for rows.Next() {
		var sh Shot
		if err := rows.Scan(&sh.ID, &sh.ReelID, &sh.OrgID, &sh.Scene, &sh.Kind, &sh.RenderState, &sh.Provider,
			&sh.ProviderJobID, &sh.OutputKey, &sh.CostUSD, &sh.Attempts); err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}
