package crawl

import (
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/models"
)

func scanGroupRows(rows *sql.Rows) ([]models.Group, error) {
	var groups []models.Group
	for rows.Next() {
		var g models.Group
		var lastScan string
		if err := rows.Scan(&g.ID, &g.Platform, &g.Name, &g.URL, &g.Active, &g.JoinState, &lastScan, &g.CreatedAt); err != nil {
			return nil, err
		}
		if lastScan != "" {
			g.LastScan, _ = time.Parse(time.RFC3339, lastScan)
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// AddGroup inserts a new group to monitor.
func (s *Store) AddGroup(g *models.Group) (int64, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO groups (org_id, platform, name, url, active, join_state) VALUES (?, ?, ?, ?, ?, ?)`,
		g.OrgID, g.Platform, g.Name, g.URL, g.Active, g.JoinState,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GroupExistsByURL checks if a group with the given URL already exists.
//
// tenant-ok: this query intentionally spans tenants — a URL is a
// global crawl-target identity, not an org-owned resource. Callers
// use this to decide "have we ever attempted to crawl this URL on
// the platform?" before scheduling. If org-scoped existence is
// required, the caller must add the org_id filter.
func (s *Store) GroupExistsByURL(url string) bool {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM groups WHERE url = ? LIMIT 1`, url).Scan(&id)
	return err == nil && id > 0
}

// GetActiveGroups returns all active groups for a platform.
//
// tenant-ok: cross-tenant aggregate read used by the crawler binary's
// scheduler pass which fans out platform-wide. Org filtering is
// applied downstream when the per-org crawl_intent fires.
func (s *Store) GetActiveGroups(platform models.Platform) ([]models.Group, error) {
	rows, err := s.db.Query(
		`SELECT id, platform, name, url, active, join_state, COALESCE(last_scan, ''), created_at FROM groups WHERE active = 1 AND platform = ? ORDER BY last_scan ASC`,
		platform,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroupRows(rows)
}

// GetAllGroups returns groups scoped to an org. orgID=0 returns all
// (admin-only callers; HTTP handlers always pass a positive org).
func (s *Store) GetAllGroups(orgID int64) ([]models.Group, error) {
	q := `SELECT id, COALESCE(org_id,1), platform, name, url, active, join_state, COALESCE(last_scan, ''), created_at FROM groups`
	var args []any
	if orgID > 0 {
		q += ` WHERE org_id = ?`
		args = append(args, orgID)
	}
	q += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []models.Group
	for rows.Next() {
		var g models.Group
		var lastScan string
		if err := rows.Scan(&g.ID, &g.OrgID, &g.Platform, &g.Name, &g.URL, &g.Active, &g.JoinState, &lastScan, &g.CreatedAt); err != nil {
			return nil, err
		}
		if lastScan != "" {
			g.LastScan, _ = time.Parse(time.RFC3339, lastScan)
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// UpdateGroupLastScan updates the last scan timestamp for a group.
func (s *Store) UpdateGroupLastScan(groupID int64) error {
	_, err := s.db.Exec(`UPDATE groups SET last_scan = CURRENT_TIMESTAMP WHERE id = ?`, groupID)
	return err
}

// ToggleGroup activates or deactivates a group.
func (s *Store) ToggleGroup(groupID int64, active bool) error {
	_, err := s.db.Exec(`UPDATE groups SET active = ? WHERE id = ?`, active, groupID)
	return err
}

// DeleteGroup removes a group.
func (s *Store) DeleteGroup(groupID int64) error {
	_, err := s.db.Exec(`DELETE FROM groups WHERE id = ?`, groupID)
	return err
}
