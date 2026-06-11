// Package telegram owns the Telegram integration control-plane tables: telegram_settings,
// telegram_bind_codes, telegram_bindings, telegram_alert_prefs, telegram_audit. Every query is
// org-scoped (tenant isolation, per [[feedback_v2_tenant_isolation_mandates]]). The domain is
// channel-neutral — alert preferences carry a channel_filter so Facebook/Taobao/1688 share one
// model. Action EXECUTION lives elsewhere and is gated by TELEGRAM_ACTIONS_ENABLED (default off);
// this store only persists bindings, preferences, and the append-only audit trail.
package telegram

import (
	"crypto/rand"
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Store provides telegram-domain data access. No cross-domain Hooks — telegram has zero
// cross-domain writes (it reads users/role from the request context, never from peer tables).
// encKey encrypts the per-org bot token at rest (AES-256-GCM via auth.Encrypt); empty = no-op (dev).
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
	encKey  string
}

// NewStore constructs a telegram Store. encKey is the app encryption key for the bot token.
func NewStore(db *sql.DB, dialect dbutil.Dialect, encKey string) *Store {
	return &Store{db: db, dialect: dialect, encKey: encKey}
}

// SetEncryptionKey updates the encryption key (mirrored from the top-level Store at boot).
func (s *Store) SetEncryptionKey(key string) { s.encKey = key }

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB { return s.db }

// Dialect returns the SQL flavour.
func (s *Store) Dialect() dbutil.Dialect { return s.dialect }

// Settings is the per-org integration row + webhook health.
type Settings struct {
	OrgID          int64        `json:"org_id"`
	Enabled        bool         `json:"enabled"`
	BotUsername    string       `json:"bot_username"`
	WebhookLastAt  sql.NullTime `json:"-"`
	WebhookLastErr string       `json:"webhook_last_err"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// AlertPrefs is the org-level alert configuration. ChannelFilter is channel-neutral; AlertTypes
// is a JSON array string owned by the caller (handler validates/serialises).
type AlertPrefs struct {
	OrgID         int64     `json:"org_id"`
	AlertsEnabled bool      `json:"alerts_enabled"`
	ChannelFilter string    `json:"channel_filter"`
	AlertTypes    string    `json:"alert_types"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// BindCode is a one-time pairing code.
type BindCode struct {
	ID        int64     `json:"id"`
	OrgID     int64     `json:"org_id"`
	UserID    int64     `json:"user_id"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}

// Binding is a user <-> telegram_user_id link.
type Binding struct {
	ID               int64        `json:"id"`
	OrgID            int64        `json:"org_id"`
	UserID           int64        `json:"user_id"`
	TelegramUserID   int64        `json:"telegram_user_id"`
	TelegramUsername string       `json:"telegram_username"`
	DisplayName      string       `json:"display_name"`
	ChatID           int64        `json:"-"` // Telegram chat id for replies/notifications; never exposed to UI
	Role             string       `json:"role"`
	AlertRecipient   bool         `json:"alert_recipient"`
	Status           string       `json:"status"`
	BoundAt          time.Time    `json:"bound_at"`
	LastCommandAt    sql.NullTime `json:"-"`
	RevokedAt        sql.NullTime `json:"-"`
}

// Counts summarises an org's bindings for the status card.
type Counts struct {
	Active          int `json:"active"`
	AlertRecipients int `json:"alert_recipients"`
}

// AuditRow is one append-only control-plane event.
type AuditRow struct {
	ID             int64     `json:"id"`
	OrgID          int64     `json:"org_id"`
	UserID         int64     `json:"user_id"`
	TelegramUserID int64     `json:"telegram_user_id"`
	Action         string    `json:"action"`
	Result         string    `json:"result"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
}

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no ambiguous chars (0/O, 1/I/L)

// GenerateCode returns an n-char single-use pairing code from an unambiguous alphabet.
// crypto/rand backed; falls back to a deterministic char only if the RNG errors (never panics).
func GenerateCode(n int) string {
	if n <= 0 {
		n = 8
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		for i := range buf {
			buf[i] = 0
		}
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = codeAlphabet[int(b)%len(codeAlphabet)]
	}
	return string(out)
}
