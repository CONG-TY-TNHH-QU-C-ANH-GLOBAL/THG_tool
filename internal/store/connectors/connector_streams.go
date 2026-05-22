// Domain: connectors (see internal/store/DOMAINS.md)
package connectors

import (
	"database/sql"
	"time"
)

func (s *Store) UpsertConnectorScreenshot(agentID, orgID, accountID int64, imageData, currentURL, fbUserID, fbDisplayName, fbUsername, fbProfileURL, streamStatus, chromeError string) error {
	if accountID <= 0 {
		accountID = 0
	}
	_, err := s.db.Exec(
		`INSERT INTO connector_screenshots
			(account_id, org_id, agent_id, image_data, current_url, fb_user_id, fb_display_name, fb_username, fb_profile_url, stream_status, chrome_error, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(org_id, account_id) DO UPDATE SET
			agent_id = excluded.agent_id,
			image_data = excluded.image_data,
			current_url = excluded.current_url,
			fb_user_id = excluded.fb_user_id,
			fb_display_name = CASE WHEN excluded.fb_display_name != '' THEN excluded.fb_display_name ELSE connector_screenshots.fb_display_name END,
			fb_username = CASE WHEN excluded.fb_username != '' THEN excluded.fb_username ELSE connector_screenshots.fb_username END,
			fb_profile_url = CASE WHEN excluded.fb_profile_url != '' THEN excluded.fb_profile_url ELSE connector_screenshots.fb_profile_url END,
			stream_status = excluded.stream_status,
			chrome_error = excluded.chrome_error,
			updated_at = CURRENT_TIMESTAMP`,
		accountID, orgID, agentID, imageData, currentURL, fbUserID, fbDisplayName, fbUsername, fbProfileURL, streamStatus, chromeError,
	)
	return err
}

func (s *Store) GetLatestConnectorScreenshot(orgID, accountID int64) (*ConnectorScreenshot, error) {
	query := `SELECT account_id, org_id, agent_id, image_data, current_url, fb_user_id,
		COALESCE(fb_display_name,''), COALESCE(fb_username,''), COALESCE(fb_profile_url,''),
		stream_status, COALESCE(chrome_error,''), updated_at
		FROM connector_screenshots WHERE org_id = ?`
	args := []any{orgID}
	if accountID > 0 {
		query += ` AND account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY updated_at DESC LIMIT 1`

	var out ConnectorScreenshot
	var updatedAt string
	err := s.db.QueryRow(query, args...).Scan(
		&out.AccountID, &out.OrgID, &out.AgentID, &out.ImageData, &out.CurrentURL, &out.FBUserID,
		&out.FBDisplayName, &out.FBUsername, &out.FBProfileURL, &out.StreamStatus, &out.ChromeError, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if parsed, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil {
		out.UpdatedAt = parsed
	} else if parsed, err := time.Parse("2006-01-02 15:04:05", updatedAt); err == nil {
		out.UpdatedAt = parsed
	}
	return &out, nil
}

func (s *Store) StopLocalSessionsForConnector(agentID, orgID int64) error {
	_, err := s.db.Exec(
		`UPDATE browser_sessions
		 SET status = 'local_stopped',
		     last_active_at = CURRENT_TIMESTAMP,
		     error_msg = ''
		 WHERE org_id = ?
		   AND status LIKE 'local_%'
		   AND account_id IN (
		   	SELECT account_id FROM connector_screenshots WHERE agent_id = ? AND org_id = ?
		   )`,
		orgID, agentID, orgID,
	)
	return err
}

func (s *Store) StopAllLocalSessionsForOrg(orgID int64) error {
	_, err := s.db.Exec(
		`UPDATE browser_sessions
		 SET status = 'local_stopped',
		     last_active_at = CURRENT_TIMESTAMP,
		     error_msg = ''
		 WHERE org_id = ?
		   AND status LIKE 'local_%'`,
		orgID,
	)
	return err
}

func (s *Store) DeleteConnectorScreenshotsByAgent(agentID, orgID int64) error {
	_, err := s.db.Exec(`DELETE FROM connector_screenshots WHERE agent_id = ? AND org_id = ?`, agentID, orgID)
	return err
}

func (s *Store) ListLocalBrowserTargets(orgID int64) ([]LocalBrowserTarget, error) {
	return s.ListLocalBrowserTargetsForConnector(orgID, 0, 0, 0)
}

func (s *Store) ListLocalBrowserTargetsForConnector(orgID, agentID, createdBy, assignedAccountID int64) ([]LocalBrowserTarget, error) {
	ownershipClause := ``
	args := []any{orgID}
	if agentID > 0 || createdBy > 0 || assignedAccountID > 0 {
		ownershipClause = ` AND (
		   (? > 0 AND a.assigned_user_id = ?)
		   OR (? > 0 AND a.id = ?)
		   OR (? > 0 AND EXISTS (
		   	SELECT 1 FROM connector_screenshots cs
		   	 WHERE cs.org_id = a.org_id AND cs.account_id = a.id AND cs.agent_id = ?
		   ))
		)`
		args = append(args, createdBy, createdBy, assignedAccountID, assignedAccountID, agentID, agentID)
	}
	rows, err := s.db.Query(
		`SELECT a.id, a.name, COALESCE(a.fb_user_id,''), COALESCE(bs.status,'local_starting')
		 FROM accounts a
		 JOIN browser_sessions bs ON bs.account_id = a.id
		 WHERE a.org_id = ?
		   AND a.platform = 'facebook'
		   AND bs.status LIKE 'local_%'
		   AND bs.status != 'local_stopped'
		   AND bs.status != 'terminated'
		`+ownershipClause+`
		 ORDER BY bs.last_active_at DESC, a.id DESC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LocalBrowserTarget
	for rows.Next() {
		var t LocalBrowserTarget
		if err := rows.Scan(&t.AccountID, &t.AccountName, &t.FBUserID, &t.Status); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
