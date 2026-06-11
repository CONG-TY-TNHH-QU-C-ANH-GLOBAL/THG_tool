package control

import (
	"strings"

	tgstore "github.com/thg/scraper/internal/store/telegram"
)

// BotInfo is the result of getMe (used to verify a token).
type BotInfo struct {
	BotID       int64
	Username    string
	DisplayName string
}

// SendResult carries the resolved chat + a sanitized Telegram error (code/description) so the
// domain can classify connect failures (bot-not-admin, channel-not-found, …) without the token.
type SendResult struct {
	ChatID   int64
	Title    string
	Username string
	ErrCode  int
	ErrDesc  string
}

// Bot abstracts a Telegram bot transport for ONE token. The domain never imports the HTTP client;
// the wiring provides a BotFactory(token) so each org's bot is built from its own (decrypted) token.
type Bot interface {
	Send(chatID int64, text string) error
	Resolve(ref, text string) (SendResult, error)
	GetMe() (BotInfo, error)
}

// BotFactory builds a Bot for a given token. Wired to the HTTP client; never holds a token itself.
type BotFactory func(token string) Bot

// Flags are the process feature flags the domain needs. GlobalToken is an OPTIONAL platform/dev bot
// (powers the shared webhook + a fallback only when AllowGlobalFallback is set). Tenant channel
// delivery uses the per-ORG token, NOT this.
type Flags struct {
	NotifyEnabled       bool
	ActionsEnabled      bool
	WebhookSecret       string
	BotUsername         string
	GlobalToken         string
	AllowGlobalFallback bool
}

// Service is the single domain service shared by the REST settings API + the webhook runtime.
type Service struct {
	store   *tgstore.Store
	factory BotFactory
	flags   Flags
}

// NewService wires the store + a per-token bot factory + feature flags.
func NewService(store *tgstore.Store, factory BotFactory, flags Flags) *Service {
	return &Service{store: store, factory: factory, flags: flags}
}

// Flags exposes the feature flags (read-only).
func (s *Service) Flags() Flags { return s.flags }

// resolveBot returns the bot for an org: its OWN token first, then the platform fallback only when
// explicitly allowed. reason="bot_token_missing" when neither is available.
func (s *Service) resolveBot(orgID int64) (Bot, string) {
	if token, ok := s.store.GetDecryptedBotToken(orgID); ok && token != "" {
		return s.factory(token), ""
	}
	if s.flags.AllowGlobalFallback && s.flags.GlobalToken != "" {
		return s.factory(s.flags.GlobalToken), ""
	}
	return nil, "bot_token_missing"
}

// globalBot returns the platform/dev bot the shared webhook belongs to (DM commands + channel_post
// connect). nil when no global token is configured.
func (s *Service) globalBot() Bot {
	if s.flags.GlobalToken == "" {
		return nil
	}
	return s.factory(s.flags.GlobalToken)
}

func (s *Service) audit(orgID, userID, tgUserID int64, action, result, meta string) {
	_ = s.store.InsertAudit(orgID, userID, tgUserID, action, result, meta)
}

// BindResult is the outcome of a /bind attempt.
type BindResult struct {
	OK     bool
	Reason string
	OrgID  int64
}

// Bind consumes a one-time code and creates an ACTIVE binding for the Telegram account. Audited.
func (s *Service) Bind(code string, tgUserID, chatID int64, username, displayName string) BindResult {
	norm := strings.ToUpper(strings.TrimSpace(code))
	orgID, userID, ok, err := s.store.ConsumeBindCode(norm)
	if err != nil || !ok {
		s.audit(0, 0, tgUserID, AuditBindFailed, "invalid_code", "")
		return BindResult{OK: false, Reason: "invalid_code"}
	}
	_, err = s.store.UpsertBinding(tgstore.Binding{
		OrgID: orgID, UserID: userID, TelegramUserID: tgUserID, ChatID: chatID,
		TelegramUsername: username, DisplayName: displayName, AlertRecipient: true, Status: "active",
	})
	if err != nil {
		s.audit(orgID, userID, tgUserID, AuditBindFailed, "error", "")
		return BindResult{OK: false, Reason: "error"}
	}
	s.audit(orgID, userID, tgUserID, AuditBindSuccess, "ok", "")
	return BindResult{OK: true, OrgID: orgID}
}

// ActiveBindings returns a Telegram account's active bindings (across orgs).
func (s *Service) ActiveBindings(tgUserID int64) ([]tgstore.Binding, bool, error) {
	bs, err := s.store.GetActiveBindingsByTelegramUser(tgUserID)
	return bs, len(bs) > 0, err
}

func (s *Service) resolveOrg(tgUserID int64) int64 {
	bs, _ := s.store.GetActiveBindingsByTelegramUser(tgUserID)
	if len(bs) > 0 {
		return bs[0].OrgID
	}
	return 0
}

// Unbind revokes every active binding for a Telegram account. Audited.
func (s *Service) Unbind(tgUserID int64) (int64, error) {
	org := s.resolveOrg(tgUserID)
	n, err := s.store.RevokeBindingsByTelegramUser(tgUserID)
	if err == nil && n > 0 {
		s.audit(org, 0, tgUserID, AuditUnbind, "ok", "")
	}
	return n, err
}
