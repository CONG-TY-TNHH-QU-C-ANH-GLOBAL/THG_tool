package telegram

import "time"

// UpdateDestinationPreferences sets the subscribed event types (JSON) + channel filter (both
// pre-validated by the caller). Org-scoped.
func (s *Store) UpdateDestinationPreferences(orgID, id int64, eventTypesJSON, channelFilter string) error {
	_, err := s.db.Exec(`UPDATE telegram_destinations
		SET event_types = ?, channel_filter = ?, updated_at = CURRENT_TIMESTAMP
		WHERE org_id = ? AND id = ?`, eventTypesJSON, channelFilter, orgID, id)
	return err
}

// SetDestinationStatus updates status (+ optional last_error). Org-scoped.
func (s *Store) SetDestinationStatus(orgID, id int64, status, lastErr string) error {
	_, err := s.db.Exec(`UPDATE telegram_destinations
		SET status = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE org_id = ? AND id = ?`, status, lastErr, orgID, id)
	return err
}

// DisableDestination soft-disables a destination (kept for audit; never hard-deleted). Org-scoped.
func (s *Store) DisableDestination(orgID, id int64) error {
	_, err := s.db.Exec(`UPDATE telegram_destinations
		SET status = 'disabled', revoked_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE org_id = ? AND id = ?`, time.Now(), orgID, id)
	return err
}

// RecordDelivery stamps the outcome of a send to a destination. A failure flips the row to
// needs_attention and records the (generic, token-free) error so the UI can surface it.
func (s *Store) RecordDelivery(orgID, id int64, ok bool, errStr string) error {
	if ok {
		_, err := s.db.Exec(`UPDATE telegram_destinations
			SET last_delivery_at = ?, last_error = '', status = 'active', updated_at = CURRENT_TIMESTAMP
			WHERE org_id = ? AND id = ?`, time.Now(), orgID, id)
		return err
	}
	_, err := s.db.Exec(`UPDATE telegram_destinations
		SET last_error = ?, status = 'needs_attention', updated_at = CURRENT_TIMESTAMP
		WHERE org_id = ? AND id = ?`, errStr, orgID, id)
	return err
}

// CountDestinations returns the connected (non-disabled) destination count for the status card.
func (s *Store) CountDestinations(orgID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM telegram_destinations
		WHERE org_id = ? AND status != 'disabled'`, orgID).Scan(&n)
	return n, err
}
