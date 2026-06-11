package control

import (
	"strings"

	tgstore "github.com/thg/scraper/internal/store/telegram"
)

// Bot abstracts the Telegram transport so the domain never imports the HTTP client (the client
// satisfies this structurally; one-directional dependency, no token in the domain). Resolve sends
// `text` to a chat reference ("@username" or numeric id) and returns the resolved chat — used to
// connect a public channel in one verified call.
type Bot interface {
	Send(chatID int64, text string) error
	Resolve(ref, text string) (chatID int64, title, username string, err error)
}

// Flags are the process feature flags the domain needs. Passed in by the wiring layer; the domain
// never imports config. ActionsEnabled MUST be false in this product.
type Flags struct {
	NotifyEnabled  bool
	ActionsEnabled bool
	WebhookSecret  string
	BotUsername    string
}

// Service is the single domain service shared by the REST settings API and the webhook runtime.
type Service struct {
	store *tgstore.Store
	tg    Bot
	flags Flags
}

// NewService wires the store + the Telegram bot transport + the feature flags.
func NewService(store *tgstore.Store, bot Bot, flags Flags) *Service {
	return &Service{store: store, tg: bot, flags: flags}
}

// Flags exposes the feature flags (read-only) for handlers that compose status.
func (s *Service) Flags() Flags { return s.flags }

// audit centralises audit writes (best-effort; a failed audit never blocks the action).
func (s *Service) audit(orgID, userID, tgUserID int64, action, result, meta string) {
	_ = s.store.InsertAudit(orgID, userID, tgUserID, action, result, meta)
}

// BindResult is the outcome of a /bind attempt.
type BindResult struct {
	OK     bool
	Reason string // invalid_code | error
	OrgID  int64
}

// Bind consumes a one-time code and creates an ACTIVE binding for the Telegram account. Audited.
// The code is normalised (trim + upper) to match the issued alphabet.
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

// ActiveBindings returns a Telegram account's active bindings (across orgs). bound=false when none.
func (s *Service) ActiveBindings(tgUserID int64) ([]tgstore.Binding, bool, error) {
	bs, err := s.store.GetActiveBindingsByTelegramUser(tgUserID)
	return bs, len(bs) > 0, err
}

// resolveOrg returns the org to attribute a command to (the account's first active binding), or 0
// when the account is unbound — used to keep audit rows tenant-scoped.
func (s *Service) resolveOrg(tgUserID int64) int64 {
	bs, _ := s.store.GetActiveBindingsByTelegramUser(tgUserID)
	if len(bs) > 0 {
		return bs[0].OrgID
	}
	return 0
}

// Unbind revokes every active binding for a Telegram account (bot-side /unbind). Audited.
func (s *Service) Unbind(tgUserID int64) (int64, error) {
	org := s.resolveOrg(tgUserID)
	n, err := s.store.RevokeBindingsByTelegramUser(tgUserID)
	if err == nil && n > 0 {
		s.audit(org, 0, tgUserID, AuditUnbind, "ok", "")
	}
	return n, err
}
