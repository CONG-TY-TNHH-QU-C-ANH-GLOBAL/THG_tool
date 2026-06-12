package store

import (
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/models"
)

// In-app notification substrate (SaaS UX Hardening PR-1). Root-store
// methods (like users/audit) — small surface, no domain subpackage yet.
// Visibility rule lives in the SQL here and ONLY here:
//   personal rows (user_id = caller)  ∪  org-wide rows (user_id = 0,
//   org = caller's org, caller is admin).

// InsertNotification writes one notification. userID 0 = org-wide
// (admin-visible); userID > 0 = personal.
func (s *Store) InsertNotification(orgID, userID int64, ntype, title, body, payloadJSON string) error {
	if payloadJSON == "" {
		payloadJSON = "{}"
	}
	_, err := s.db.Exec(
		`INSERT INTO notifications (org_id, user_id, type, title, body, payload_json)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		orgID, userID, ntype, title, body, payloadJSON,
	)
	return err
}

// ListNotificationsForUser returns the caller's visible notifications,
// newest first. isAdmin additionally surfaces the caller-org's
// org-wide rows.
func (s *Store) ListNotificationsForUser(orgID, userID int64, isAdmin bool, limit int) ([]models.Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := `SELECT id, org_id, user_id, type, title, body, payload_json,
	             COALESCE(read_at, ''), created_at
	        FROM notifications WHERE user_id = ?`
	args := []any{userID}
	if isAdmin && orgID > 0 {
		q += ` UNION ALL
		       SELECT id, org_id, user_id, type, title, body, payload_json,
		              COALESCE(read_at, ''), created_at
		         FROM notifications WHERE user_id = 0 AND org_id = ?`
		args = append(args, orgID)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		var readAt string
		if err := rows.Scan(&n.ID, &n.OrgID, &n.UserID, &n.Type, &n.Title, &n.Body,
			&n.PayloadJSON, &readAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		if readAt != "" {
			if t, err := time.Parse(time.RFC3339, readAt); err == nil {
				n.ReadAt = &t
			} else if t, err := time.Parse("2006-01-02 15:04:05", readAt); err == nil {
				n.ReadAt = &t
			}
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CountUnreadNotifications powers the bell badge.
func (s *Store) CountUnreadNotifications(orgID, userID int64, isAdmin bool) (int, error) {
	q := `SELECT COUNT(*) FROM notifications WHERE read_at IS NULL AND (user_id = ?`
	args := []any{userID}
	if isAdmin && orgID > 0 {
		q += ` OR (user_id = 0 AND org_id = ?)`
		args = append(args, orgID)
	}
	q += `)`
	var n int
	err := s.db.QueryRow(q, args...).Scan(&n)
	return n, err
}

// MarkNotificationRead marks one visible notification read. The WHERE
// clause enforces the same visibility rule as the list — a caller can
// never mark another user's (or another org's) notification.
func (s *Store) MarkNotificationRead(id, orgID, userID int64, isAdmin bool) error {
	q := `UPDATE notifications SET read_at = CURRENT_TIMESTAMP
	       WHERE id = ? AND read_at IS NULL AND (user_id = ?`
	args := []any{id, userID}
	if isAdmin && orgID > 0 {
		q += ` OR (user_id = 0 AND org_id = ?)`
		args = append(args, orgID)
	}
	q += `)`
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MarkAllNotificationsRead clears the caller's visible unread set.
func (s *Store) MarkAllNotificationsRead(orgID, userID int64, isAdmin bool) error {
	q := `UPDATE notifications SET read_at = CURRENT_TIMESTAMP
	       WHERE read_at IS NULL AND (user_id = ?`
	args := []any{userID}
	if isAdmin && orgID > 0 {
		q += ` OR (user_id = 0 AND org_id = ?)`
		args = append(args, orgID)
	}
	q += `)`
	_, err := s.db.Exec(q, args...)
	return err
}
