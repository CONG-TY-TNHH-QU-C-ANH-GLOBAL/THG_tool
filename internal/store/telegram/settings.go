package telegram

import "database/sql"

// GetSettings returns the org's integration settings, or a zero-value (disabled) Settings when no
// row exists yet — callers treat "no row" as "not connected".
func (s *Store) GetSettings(orgID int64) (Settings, error) {
	out := Settings{OrgID: orgID}
	row := s.db.QueryRow(`
		SELECT enabled, bot_username, webhook_last_at, webhook_last_err, updated_at
		FROM telegram_settings WHERE org_id = ?`, orgID)
	var enabled int
	err := row.Scan(&enabled, &out.BotUsername, &out.WebhookLastAt, &out.WebhookLastErr, &out.UpdatedAt)
	if err == sql.ErrNoRows {
		return out, nil
	}
	out.Enabled = enabled != 0
	return out, err
}

// SetEnabled upserts the org's integration on/off flag (admin action).
func (s *Store) SetEnabled(orgID int64, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO telegram_settings (org_id, enabled, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(org_id) DO UPDATE SET enabled = excluded.enabled, updated_at = CURRENT_TIMESTAMP`,
		orgID, v)
	return err
}

// GetAlertPrefs returns the org's alert preferences, or sensible defaults when no row exists.
func (s *Store) GetAlertPrefs(orgID int64) (AlertPrefs, error) {
	out := AlertPrefs{OrgID: orgID, AlertsEnabled: true, ChannelFilter: "all", AlertTypes: "[]"}
	row := s.db.QueryRow(`
		SELECT alerts_enabled, channel_filter, alert_types, updated_at
		FROM telegram_alert_prefs WHERE org_id = ?`, orgID)
	var enabled int
	err := row.Scan(&enabled, &out.ChannelFilter, &out.AlertTypes, &out.UpdatedAt)
	if err == sql.ErrNoRows {
		return out, nil
	}
	out.AlertsEnabled = enabled != 0
	return out, err
}

// UpsertAlertPrefs writes the org's alert preferences (admin action). alertTypes must be a valid
// JSON array string; channelFilter must already be validated by the caller.
func (s *Store) UpsertAlertPrefs(orgID int64, alertsEnabled bool, channelFilter, alertTypes string) error {
	v := 0
	if alertsEnabled {
		v = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO telegram_alert_prefs (org_id, alerts_enabled, channel_filter, alert_types, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(org_id) DO UPDATE SET
			alerts_enabled = excluded.alerts_enabled,
			channel_filter = excluded.channel_filter,
			alert_types    = excluded.alert_types,
			updated_at     = CURRENT_TIMESTAMP`,
		orgID, v, channelFilter, alertTypes)
	return err
}
