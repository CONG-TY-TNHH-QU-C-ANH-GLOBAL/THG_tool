package config

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Telegram
	TelegramBotToken  string
	TelegramAdminChat int64
	TelegramOrgID     int64
	// Telegram integration feature flags (control-plane). Default-safe: the bot + notifications
	// are opt-in, and action EXECUTION is OFF by default and must stay off until reliability is
	// validated (spec: specs/OMNICHANNEL_SALES_COPILOT_TELEGRAM_TRACK.md).
	TelegramBotEnabled     bool   // TELEGRAM_BOT_ENABLED
	TelegramNotifyEnabled  bool   // TELEGRAM_NOTIFY_ENABLED
	TelegramActionsEnabled bool   // TELEGRAM_ACTIONS_ENABLED (must default false)
	TelegramWebhookSecret  string // TELEGRAM_WEBHOOK_SECRET (validates inbound webhook calls)
	// TELEGRAM_ALLOW_GLOBAL_FALLBACK: when true, tenant channel delivery may fall back to the
	// platform TELEGRAM_BOT_TOKEN if an org has not connected its own bot. Default false — enterprise
	// tenants connect their OWN bot (the token is a customer secret).
	TelegramAllowGlobalFallback bool

	// AI (OpenAI only).
	//
	// Two-model split: classifier runs on every crawled post (high volume,
	// strict JSON output via response_format: json_schema — accuracy beats
	// raw quality), so it lives on a cheap/fast model. Comment + inbox
	// generation runs once per outbound message and is user-facing, so it
	// gets the strong model.
	OpenAIAPIKey          string
	OpenAIClassifierModel string // UniversalClassify + price extraction. Cheap+fast: gpt-4o-mini / gpt-5.4-mini.
	OpenAICommentModel    string // Comments, inbox, follow-up, job posts, agent reasoning. Strong: gpt-4.1 / gpt-5.4.
	AgentBrainURL         string // optional Python sidecar planner endpoint base URL
	AgentBrainTimeout     int    // milliseconds

	// Security
	APISecret      string // DEPRECATED: legacy API key; replaced by JWT auth
	JWTSecret      string // HMAC-SHA256 signing secret for access tokens (REQUIRED in production)
	EncryptionKey  string // AES-256-GCM key for encrypting sensitive DB fields (REQUIRED in production)
	AllowedOrigins string // comma-separated allowed CORS origins (empty = localhost only)

	// Admin bootstrap (create first admin user on cold start)
	AdminEmail    string
	AdminPassword string
	AdminName     string

	// Proxy
	ProxyList []string

	// Chrome
	ChromePath string
	ProfileDir string // persistent Chrome profile directory
	Headless   bool   // true = no display server; auto-detected or forced via HEADLESS=true
	ServerHost string // public hostname/IP used in SSH tunnel instructions
	SSHPort    int    // SSH port for tunnel instructions (default 22)

	// Browser debug ports (VNC removed 2026-07-01 — product direction is
	// CDP/extension readiness signals, not VNC screen viewing)
	CDPPort    int // Chrome DevTools Protocol debug port (default 9222)
	DisplayNum int // X11 display number for Xvfb (default 99)

	// Google OAuth
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string

	// Email invites
	SMTPHost       string
	SMTPPort       int
	SMTPUsername   string
	SMTPPassword   string
	SMTPFromEmail  string
	SMTPFromName   string
	SMTPTLS        bool
	SMTPStartTLS   bool
	SMTPSkipVerify bool
	AppBaseURL     string

	// Web
	WebPort int

	// Database
	DBPath        string
	BackupEnabled bool // auto-backup SQLite daily

	// Scraper
	MaxWorkers      int
	ScrollTimeout   int // seconds
	ScanIntervalMin int // minutes

	// Lead lifecycle / auto-archive (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md)
	StaleAfterDays             int // days of no activity before a lead reads as stale
	ArchiveAfterDays           int // days of no activity before the sweep auto-archives
	EvidenceRetentionDays      int // retention for execution evidence blobs (compaction; ledger kept)
	RawCrawlRetentionDays      int // retention for raw crawl payload (compaction; ledger kept)
	ArchiveIntervalMin         int // auto-archive sweep cadence in minutes
	VerificationCooldownMin    int // how long a submitted-unverified comment holds a lead (waiting_verification)
	CommentReverifyDelayMin    int // wait this long after submit before scheduling an async reverify
	CommentReverifyIntervalMin int // async reverify scheduler cadence in minutes
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	cfg := &Config{
		TelegramBotToken:            getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramAdminChat:           getEnvInt64("TELEGRAM_ADMIN_CHAT_ID", 0),
		TelegramOrgID:               getEnvInt64("TELEGRAM_ORG_ID", 1),
		TelegramBotEnabled:          getEnvBool("TELEGRAM_BOT_ENABLED", false),
		TelegramNotifyEnabled:       getEnvBool("TELEGRAM_NOTIFY_ENABLED", true),
		TelegramActionsEnabled:      getEnvBool("TELEGRAM_ACTIONS_ENABLED", false),
		TelegramWebhookSecret:       getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
		TelegramAllowGlobalFallback: getEnvBool("TELEGRAM_ALLOW_GLOBAL_FALLBACK", false),
		OpenAIAPIKey:                getEnv("OPENAI_API_KEY", ""),
		// OPENAI_CLASSIFIER_MODEL is the canonical name; OPENAI_MODEL is kept as a
		// legacy alias so existing /etc/thg-scraper/env files on production VPS
		// don't break on the next deploy. Drop the alias once VPS env is updated.
		OpenAIClassifierModel: getEnv("OPENAI_CLASSIFIER_MODEL", getEnv("OPENAI_MODEL", "gpt-5.0-mini")),
		// Default is the fast chat model gpt-4.1 — NOT a reasoning model. A short
		// Facebook comment/inbox reply does not benefit from gpt-5.0 reasoning,
		// which spends hidden reasoning tokens and routinely pushes a single
		// generation past the 25s genCtx timeout ("context deadline exceeded").
		// gpt-4.1 returns in a few seconds, eliminating those timeouts, at lower
		// cost and equivalent quality for this length. Override with
		// OPENAI_COMMENT_MODEL (e.g. gpt-5.0) if you want reasoning — and raise the
		// generation timeout to match. callOpenAI omits temperature for gpt-5*/o*
		// automatically when such a model is configured.
		OpenAICommentModel:         getEnv("OPENAI_COMMENT_MODEL", "gpt-4.1"),
		AgentBrainURL:              getEnv("AGENT_BRAIN_URL", ""),
		AgentBrainTimeout:          getEnvInt("AGENT_BRAIN_TIMEOUT_MS", 1500),
		APISecret:                  getEnv("API_SECRET", ""),
		JWTSecret:                  getEnv("JWT_SECRET", ""),
		EncryptionKey:              getEnv("ENCRYPTION_KEY", ""),
		AllowedOrigins:             getEnv("ALLOWED_ORIGINS", ""),
		AdminEmail:                 getEnv("ADMIN_EMAIL", ""),
		AdminPassword:              getEnv("ADMIN_PASSWORD", ""),
		AdminName:                  getEnv("ADMIN_NAME", "Admin"),
		ChromePath:                 getEnv("CHROME_PATH", ""),
		ProfileDir:                 getEnv("PROFILE_DIR", "data/profiles"),
		Headless:                   detectHeadless(),
		ServerHost:                 getEnv("SERVER_HOST", ""),
		SSHPort:                    getEnvInt("SSH_PORT", 22),
		CDPPort:                    getEnvInt("CDP_PORT", 9222),
		DisplayNum:                 getEnvInt("DISPLAY_NUM", 99),
		WebPort:                    getEnvInt("WEB_PORT", 8080),
		DBPath:                     getEnv("DB_PATH", "data/scraper.db"),
		BackupEnabled:              getEnv("BACKUP_ENABLED", "true") == "true",
		MaxWorkers:                 getEnvInt("MAX_WORKERS", 1),
		ScrollTimeout:              getEnvInt("SCROLL_TIMEOUT_SEC", 60),
		ScanIntervalMin:            getEnvInt("SCAN_INTERVAL_MIN", 30),
		StaleAfterDays:             getEnvInt("LEAD_STALE_AFTER_DAYS", 14),
		ArchiveAfterDays:           getEnvInt("LEAD_ARCHIVE_AFTER_DAYS", 30),
		EvidenceRetentionDays:      getEnvInt("LEAD_EVIDENCE_RETENTION_DAYS", 14),
		RawCrawlRetentionDays:      getEnvInt("LEAD_RAW_CRAWL_RETENTION_DAYS", 90),
		ArchiveIntervalMin:         getEnvInt("LEAD_ARCHIVE_INTERVAL_MIN", 360),
		VerificationCooldownMin:    getEnvInt("LEAD_VERIFICATION_COOLDOWN_MIN", 30),
		CommentReverifyDelayMin:    getEnvInt("COMMENT_REVERIFY_DELAY_MIN", 3),
		CommentReverifyIntervalMin: getEnvInt("COMMENT_REVERIFY_INTERVAL_MIN", 2),
		GoogleClientID:             getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:         getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURI:          getEnv("GOOGLE_REDIRECT_URI", ""),
		SMTPHost:                   getEnv("SMTP_HOST", ""),
		SMTPPort:                   getEnvInt("SMTP_PORT", 587),
		SMTPUsername:               getEnv("SMTP_USERNAME", ""),
		SMTPPassword:               getEnv("SMTP_PASSWORD", ""),
		SMTPFromEmail:              getEnv("SMTP_FROM_EMAIL", ""),
		SMTPFromName:               getEnv("SMTP_FROM_NAME", "THG AutoFlow"),
		SMTPTLS:                    getEnvBool("SMTP_TLS", false),
		SMTPStartTLS:               getEnvBool("SMTP_STARTTLS", true),
		SMTPSkipVerify:             getEnvBool("SMTP_SKIP_VERIFY", false),
		AppBaseURL:                 getEnv("APP_BASE_URL", getEnv("PUBLIC_APP_URL", getEnv("NEXT_PUBLIC_SITE_URL", ""))),
	}

	if proxyStr := getEnv("PROXY_LIST", ""); proxyStr != "" {
		cfg.ProxyList = strings.Split(proxyStr, ",")
		for i := range cfg.ProxyList {
			cfg.ProxyList[i] = strings.TrimSpace(cfg.ProxyList[i])
		}
	}

	return cfg
}

// IsProduction reports whether the runtime should refuse to boot with
// missing/insecure secrets. The check is conservative: any explicit
// APP_ENV/ENV value of "prod" or "production" enables strict mode.
//
// Localhost development still runs fine because the env vars are unset.
func (c *Config) IsProduction() bool {
	for _, key := range []string{"APP_ENV", "ENV", "GO_ENV"} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
		case "prod", "production":
			return true
		}
	}
	return false
}

// MustValidateProductionSecrets returns an error when production-critical
// secrets are missing. Callers should log.Fatal on the returned error so
// the server never starts with cookies stored in plaintext or JWT auth
// disabled.
//
// The intent is fail-fast at boot, not surprise plaintext storage three
// weeks into operation. Pair with APP_ENV=production in deployment.
func (c *Config) MustValidateProductionSecrets() error {
	if !c.IsProduction() {
		return nil
	}
	var missing []string
	if strings.TrimSpace(c.JWTSecret) == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if strings.TrimSpace(c.EncryptionKey) == "" {
		missing = append(missing, "ENCRYPTION_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("production startup blocked: missing required secrets: %s", strings.Join(missing, ", "))
	}
	return nil
}

// detectHeadless returns true when forced via HEADLESS=true env var, or when
// running on Linux without an X11/Wayland display (i.e. a headless VPS).
func detectHeadless() bool {
	if strings.ToLower(os.Getenv("HEADLESS")) == "true" {
		return true
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return true
	}
	return false
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y", "on":
			return true
		case "0", "false", "no", "n", "off":
			return false
		}
	}
	return fallback
}
