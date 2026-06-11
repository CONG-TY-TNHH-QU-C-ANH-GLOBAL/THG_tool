package telegram

import (
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/auth"
)

// BotCredential is an org's Telegram bot. The token is NEVER in this struct's JSON (token_encrypted
// has no json tag exposure) — only safe fields are marshalled.
type BotCredential struct {
	OrgID          int64        `json:"-"`
	BotID          int64        `json:"bot_id"`
	BotUsername    string       `json:"bot_username"`
	BotDisplayName string       `json:"bot_display_name"`
	TokenLast4     string       `json:"token_last4"`
	Status         string       `json:"status"`
	LastVerifiedAt sql.NullTime `json:"-"`
	LastError      string       `json:"last_error"`
}

func last4(token string) string {
	if len(token) <= 4 {
		return token
	}
	return token[len(token)-4:]
}

// UpsertBotCredential stores (or replaces) an org's bot, encrypting the token at rest. Called only
// after the token is verified via getMe. plaintext token is encrypted here and never persisted raw.
func (s *Store) UpsertBotCredential(orgID, userID int64, token string, botID int64, username, display string) error {
	enc, err := auth.Encrypt(token, s.encKey)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO telegram_bot_credentials
		(org_id, bot_id, bot_username, bot_display_name, token_encrypted, token_last4, status,
		 created_by_user_id, last_verified_at)
		VALUES (?, ?, ?, ?, ?, ?, 'active', ?, ?)
		ON CONFLICT(org_id) DO UPDATE SET
			bot_id = excluded.bot_id, bot_username = excluded.bot_username,
			bot_display_name = excluded.bot_display_name, token_encrypted = excluded.token_encrypted,
			token_last4 = excluded.token_last4, status = 'active', last_error = '',
			last_verified_at = excluded.last_verified_at, revoked_at = NULL, updated_at = CURRENT_TIMESTAMP`,
		orgID, botID, username, display, enc, last4(token), userID, time.Now())
	return err
}

// GetBotCredential returns an org's bot METADATA (no token). nil when none.
func (s *Store) GetBotCredential(orgID int64) (*BotCredential, error) {
	row := s.db.QueryRow(`SELECT bot_id, bot_username, bot_display_name, token_last4, status,
		last_verified_at, last_error FROM telegram_bot_credentials WHERE org_id = ?`, orgID)
	var b BotCredential
	b.OrgID = orgID
	err := row.Scan(&b.BotID, &b.BotUsername, &b.BotDisplayName, &b.TokenLast4, &b.Status, &b.LastVerifiedAt, &b.LastError)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &b, err
}

// GetDecryptedBotToken returns the org's plaintext bot token. INTERNAL USE ONLY (the Telegram send/
// verify path) — never expose this value to the UI, logs, or audit. ok=false when no active token.
func (s *Store) GetDecryptedBotToken(orgID int64) (string, bool) {
	var enc, status string
	err := s.db.QueryRow(`SELECT token_encrypted, status FROM telegram_bot_credentials
		WHERE org_id = ?`, orgID).Scan(&enc, &status)
	if err != nil || enc == "" || status == "revoked" {
		return "", false
	}
	token, err := auth.Decrypt(enc, s.encKey)
	if err != nil || token == "" {
		return "", false
	}
	return token, true
}

// SetBotStatus updates a bot's status + last_error (e.g. invalid after a failed re-verify).
func (s *Store) SetBotStatus(orgID int64, status, lastErr string) error {
	_, err := s.db.Exec(`UPDATE telegram_bot_credentials
		SET status = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE org_id = ?`, status, lastErr, orgID)
	return err
}

// RevokeBotCredential wipes the token and marks the bot revoked (append-only intent; the row is
// kept for audit but the secret is destroyed).
func (s *Store) RevokeBotCredential(orgID int64) error {
	_, err := s.db.Exec(`UPDATE telegram_bot_credentials
		SET status = 'revoked', token_encrypted = '', revoked_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE org_id = ?`, time.Now(), orgID)
	return err
}
