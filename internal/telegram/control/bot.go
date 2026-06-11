package control

import (
	"strings"

	tgstore "github.com/thg/scraper/internal/store/telegram"
)

// SaveBotToken verifies a customer-supplied bot token via getMe and, if valid, stores it ENCRYPTED
// for the org. The plaintext token never leaves this call (factory -> client -> store.Encrypt).
// Returns a typed reason on failure. Audited (token never in the audit metadata).
func (s *Service) SaveBotToken(orgID, userID int64, token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "bot_token_missing", false
	}
	info, err := s.factory(token).GetMe()
	if err != nil || info.BotID == 0 {
		s.audit(orgID, userID, 0, AuditBotSaved, "bot_token_invalid", "")
		return "bot_token_invalid", false
	}
	if err := s.store.UpsertBotCredential(orgID, userID, token, info.BotID, info.Username, info.DisplayName); err != nil {
		return "error", false
	}
	s.audit(orgID, userID, 0, AuditBotSaved, "ok", "@"+info.Username)
	return "", true
}

// VerifyBot re-checks the stored token against getMe and refreshes status/last_verified_at. Audited.
func (s *Service) VerifyBot(orgID, userID int64) (string, bool) {
	token, ok := s.store.GetDecryptedBotToken(orgID)
	if !ok {
		return "bot_token_missing", false
	}
	info, err := s.factory(token).GetMe()
	if err != nil || info.BotID == 0 {
		_ = s.store.SetBotStatus(orgID, "invalid", "getMe failed")
		s.audit(orgID, userID, 0, AuditBotVerified, "bot_token_invalid", "")
		return "bot_token_invalid", false
	}
	_ = s.store.UpsertBotCredential(orgID, userID, token, info.BotID, info.Username, info.DisplayName)
	s.audit(orgID, userID, 0, AuditBotVerified, "ok", "@"+info.Username)
	return "", true
}

// BotStatus returns the org's bot metadata (safe fields only; nil when no bot is configured).
func (s *Service) BotStatus(orgID int64) (*tgstore.BotCredential, error) {
	return s.store.GetBotCredential(orgID)
}

// RevokeBot wipes the stored token and marks the bot revoked. Audited.
func (s *Service) RevokeBot(orgID, userID int64) error {
	err := s.store.RevokeBotCredential(orgID)
	if err == nil {
		s.audit(orgID, userID, 0, AuditBotRevoked, "ok", "")
	}
	return err
}

// classifyTelegramError maps a Telegram error_code + description to a sanitized, specific reason.
// The description is only inspected for known phrases; it is never returned verbatim (no secrets,
// but keep the contract tight).
func classifyTelegramError(code int, desc string) string {
	d := strings.ToLower(desc)
	switch {
	case code == 400:
		return "channel_not_found_or_username_invalid"
	case code == 403:
		if strings.Contains(d, "not enough rights") || strings.Contains(d, "post messages") || strings.Contains(d, "administrator") {
			return "bot_lacks_post_permission"
		}
		return "bot_not_channel_admin"
	case code != 0:
		return "telegram_api_error"
	default:
		return "channel_not_found_or_username_invalid"
	}
}
