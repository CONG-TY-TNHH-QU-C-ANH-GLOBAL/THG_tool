package telegram

import (
	"database/sql"
	"time"
)

// Destination is a place automation events are delivered (primarily a Telegram channel). Org-scoped.
type Destination struct {
	ID                int64        `json:"id"`
	OrgID             int64        `json:"-"`
	DestinationType   string       `json:"destination_type"`
	ChatID            int64        `json:"-"` // never exposed to the UI
	Title             string       `json:"title"`
	Username          string       `json:"username"`
	InviteLink        string       `json:"invite_link"`
	Status            string       `json:"status"`
	EventTypes        string       `json:"-"` // JSON array; surfaced decoded by the handler
	ChannelFilter     string       `json:"channel_filter"`
	DeliveryMode      string       `json:"delivery_mode"`
	ConnectedByUserID int64        `json:"connected_by_user_id"`
	LastDeliveryAt    sql.NullTime `json:"-"`
	LastError         string       `json:"last_error"`
	CreatedAt         time.Time    `json:"created_at"`
}

const destCols = `id, org_id, destination_type, chat_id, title, username, invite_link, status,
	event_types, channel_filter, delivery_mode, connected_by_user_id, last_delivery_at, last_error, created_at`

func scanDest(rows *sql.Rows) (Destination, error) {
	var d Destination
	err := rows.Scan(&d.ID, &d.OrgID, &d.DestinationType, &d.ChatID, &d.Title, &d.Username,
		&d.InviteLink, &d.Status, &d.EventTypes, &d.ChannelFilter, &d.DeliveryMode,
		&d.ConnectedByUserID, &d.LastDeliveryAt, &d.LastError, &d.CreatedAt)
	return d, err
}

func collectDests(rows *sql.Rows, err error) ([]Destination, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Destination{}
	for rows.Next() {
		d, e := scanDest(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpsertDestination creates a destination, or RE-CONNECTS (reactivates + refreshes title/username)
// an existing (org, chat_id). event_types/channel_filter are only set on first create.
func (s *Store) UpsertDestination(d Destination) (int64, error) {
	if d.Status == "" {
		d.Status = "active"
	}
	if d.EventTypes == "" {
		d.EventTypes = "[]"
	}
	if d.ChannelFilter == "" {
		d.ChannelFilter = "all"
	}
	res, err := s.db.Exec(`INSERT INTO telegram_destinations
		(org_id, destination_type, chat_id, title, username, invite_link, status, event_types,
		 channel_filter, delivery_mode, connected_by_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'immediate', ?)
		ON CONFLICT(org_id, chat_id) DO UPDATE SET
			title = excluded.title, username = excluded.username, status = 'active',
			revoked_at = NULL, last_error = '', updated_at = CURRENT_TIMESTAMP`,
		d.OrgID, d.DestinationType, d.ChatID, d.Title, d.Username, d.InviteLink, d.Status,
		d.EventTypes, d.ChannelFilter, d.ConnectedByUserID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListDestinations returns an org's destinations (newest first).
func (s *Store) ListDestinations(orgID int64) ([]Destination, error) {
	rows, err := s.db.Query(`SELECT `+destCols+` FROM telegram_destinations
		WHERE org_id = ? ORDER BY created_at DESC`, orgID)
	return collectDests(rows, err)
}

// ListActiveDestinations returns the org's DELIVERABLE destinations (notifier targets) — anything
// not 'disabled'. A 'needs_attention' destination (a prior transient send failure) keeps receiving
// notifications; the status is a soft warning surfaced in the UI, not a delivery stop.
func (s *Store) ListActiveDestinations(orgID int64) ([]Destination, error) {
	rows, err := s.db.Query(`SELECT `+destCols+` FROM telegram_destinations
		WHERE org_id = ? AND status != 'disabled'`, orgID)
	return collectDests(rows, err)
}

// GetDestination loads one destination within an org (nil when not found).
func (s *Store) GetDestination(orgID, id int64) (*Destination, error) {
	rows, err := s.db.Query(`SELECT `+destCols+` FROM telegram_destinations
		WHERE org_id = ? AND id = ?`, orgID, id)
	list, err := collectDests(rows, err)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}
