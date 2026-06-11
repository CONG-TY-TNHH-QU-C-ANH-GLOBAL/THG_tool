package telegram

import (
	"database/sql"
	"time"
)

// CreateBindCode inserts a one-time pairing code for (org, user) with the given TTL.
func (s *Store) CreateBindCode(orgID, userID int64, code string, ttl time.Duration) (BindCode, error) {
	expires := time.Now().Add(ttl)
	res, err := s.db.Exec(`
		INSERT INTO telegram_bind_codes (org_id, user_id, code, expires_at, used)
		VALUES (?, ?, ?, ?, 0)`, orgID, userID, code, expires)
	if err != nil {
		return BindCode{}, err
	}
	id, _ := res.LastInsertId()
	return BindCode{ID: id, OrgID: orgID, UserID: userID, Code: code, ExpiresAt: expires}, nil
}

// ConsumeBindCode marks a still-valid, unused code as used and returns its (org, user). It is the
// bot side of binding (the web app only issues codes) — used by the bot's /bind handler later.
// Returns ok=false when the code is missing, used, or expired.
func (s *Store) ConsumeBindCode(code string) (orgID, userID int64, ok bool, err error) {
	// Compare expiry against a Go-encoded time (NOT SQLite CURRENT_TIMESTAMP): expires_at is
	// stored via the Go driver, and mixing the two encodings (RFC3339 'T'/offset vs space-UTC)
	// breaks the lexical comparison and lets expired codes pass.
	row := s.db.QueryRow(`
		SELECT org_id, user_id FROM telegram_bind_codes
		WHERE code = ? AND used = 0 AND expires_at > ?`, code, time.Now())
	if err = row.Scan(&orgID, &userID); err == sql.ErrNoRows {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	if _, err = s.db.Exec(`UPDATE telegram_bind_codes SET used = 1 WHERE code = ?`, code); err != nil {
		return 0, 0, false, err
	}
	return orgID, userID, true, nil
}

// UpsertBinding inserts an active binding (the bot side of /bind, after ConsumeBindCode). Status
// defaults to "active". Returns the new row id.
func (s *Store) UpsertBinding(b Binding) (int64, error) {
	recipient := 0
	if b.AlertRecipient {
		recipient = 1
	}
	status := b.Status
	if status == "" {
		status = "active"
	}
	res, err := s.db.Exec(`INSERT INTO telegram_bindings
		(org_id, user_id, telegram_user_id, telegram_username, display_name, chat_id, role, alert_recipient, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.OrgID, b.UserID, b.TelegramUserID, b.TelegramUsername, b.DisplayName, b.ChatID, b.Role, recipient, status)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const bindingCols = `id, org_id, user_id, telegram_user_id, telegram_username, display_name,
	chat_id, role, alert_recipient, status, bound_at, last_command_at, revoked_at`

func scanBinding(rows *sql.Rows) (Binding, error) {
	var b Binding
	var recipient int
	err := rows.Scan(&b.ID, &b.OrgID, &b.UserID, &b.TelegramUserID, &b.TelegramUsername,
		&b.DisplayName, &b.ChatID, &b.Role, &recipient, &b.Status, &b.BoundAt, &b.LastCommandAt, &b.RevokedAt)
	b.AlertRecipient = recipient != 0
	return b, err
}

// ListBindings returns all bindings for an org (admin view), newest first.
func (s *Store) ListBindings(orgID int64) ([]Binding, error) {
	rows, err := s.db.Query(`SELECT `+bindingCols+` FROM telegram_bindings
		WHERE org_id = ? ORDER BY bound_at DESC`, orgID)
	return collectBindings(rows, err)
}

// ListBindingsByUser returns only the caller's own bindings (member view).
func (s *Store) ListBindingsByUser(orgID, userID int64) ([]Binding, error) {
	rows, err := s.db.Query(`SELECT `+bindingCols+` FROM telegram_bindings
		WHERE org_id = ? AND user_id = ? ORDER BY bound_at DESC`, orgID, userID)
	return collectBindings(rows, err)
}

func collectBindings(rows *sql.Rows, err error) ([]Binding, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Binding{}
	for rows.Next() {
		b, e := scanBinding(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetBinding loads one binding by id within an org (nil when not found).
func (s *Store) GetBinding(orgID, id int64) (*Binding, error) {
	rows, err := s.db.Query(`SELECT `+bindingCols+` FROM telegram_bindings
		WHERE org_id = ? AND id = ?`, orgID, id)
	list, err := collectBindings(rows, err)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

// RevokeBinding flips a binding to revoked (append-only intent: never hard-deleted). Scoped by
// org so a caller can never revoke another tenant's binding.
func (s *Store) RevokeBinding(orgID, id int64) error {
	_, err := s.db.Exec(`UPDATE telegram_bindings
		SET status = 'revoked', revoked_at = CURRENT_TIMESTAMP
		WHERE org_id = ? AND id = ? AND status = 'active'`, orgID, id)
	return err
}

// CountBindings returns active + alert-recipient counts for the status card.
func (s *Store) CountBindings(orgID int64) (Counts, error) {
	var c Counts
	err := s.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'active' AND alert_recipient = 1 THEN 1 ELSE 0 END), 0)
		FROM telegram_bindings WHERE org_id = ?`, orgID).Scan(&c.Active, &c.AlertRecipients)
	return c, err
}
