package telegram

import "time"

// Runtime-side queries used by the webhook bot + notifier. A Telegram user is identified ONLY by
// their telegram_user_id (the webhook has no JWT/org context), so these look up by that id and may
// span multiple orgs the same Telegram account is bound to. All remain append-only on revoke.

// GetActiveBindingsByTelegramUser returns every ACTIVE binding for a Telegram account (one per org
// it has bound). Empty slice when the account is unbound.
func (s *Store) GetActiveBindingsByTelegramUser(telegramUserID int64) ([]Binding, error) {
	rows, err := s.db.Query(`SELECT `+bindingCols+` FROM telegram_bindings
		WHERE telegram_user_id = ? AND status = 'active' ORDER BY bound_at DESC`, telegramUserID)
	return collectBindings(rows, err)
}

// UpdateLastCommand stamps last_command_at for all active bindings of a Telegram account. Best
// effort — a missing binding is a no-op.
func (s *Store) UpdateLastCommand(telegramUserID int64) error {
	_, err := s.db.Exec(`UPDATE telegram_bindings SET last_command_at = ?
		WHERE telegram_user_id = ? AND status = 'active'`, time.Now(), telegramUserID)
	return err
}

// RevokeBindingsByTelegramUser revokes every active binding for a Telegram account (the bot-side
// /unbind). Returns the number of bindings revoked.
func (s *Store) RevokeBindingsByTelegramUser(telegramUserID int64) (int64, error) {
	res, err := s.db.Exec(`UPDATE telegram_bindings
		SET status = 'revoked', revoked_at = ?
		WHERE telegram_user_id = ? AND status = 'active'`, time.Now(), telegramUserID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ListAlertRecipients returns the active, alert-recipient bindings for an org (notifier targets).
func (s *Store) ListAlertRecipients(orgID int64) ([]Binding, error) {
	rows, err := s.db.Query(`SELECT `+bindingCols+` FROM telegram_bindings
		WHERE org_id = ? AND status = 'active' AND alert_recipient = 1`, orgID)
	return collectBindings(rows, err)
}
