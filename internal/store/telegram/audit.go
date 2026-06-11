package telegram

// InsertAudit appends one control-plane event. Append-only: audit rows are never updated or
// deleted (per [[feedback_append_only_correction_events]]). metadata is a caller-owned JSON/string.
func (s *Store) InsertAudit(orgID, userID, telegramUserID int64, action, result, metadata string) error {
	_, err := s.db.Exec(`
		INSERT INTO telegram_audit (org_id, user_id, telegram_user_id, action, result, metadata)
		VALUES (?, ?, ?, ?, ?, ?)`, orgID, userID, telegramUserID, action, result, metadata)
	return err
}

// ListAudit returns the org's most-recent audit events (newest first), capped at limit.
func (s *Store) ListAudit(orgID int64, limit int) ([]AuditRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT id, org_id, user_id, telegram_user_id, action, result, metadata, created_at
		FROM telegram_audit WHERE org_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditRow{}
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.ID, &a.OrgID, &a.UserID, &a.TelegramUserID, &a.Action,
			&a.Result, &a.Metadata, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
