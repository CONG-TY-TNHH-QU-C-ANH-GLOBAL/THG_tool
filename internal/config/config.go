package config

import (
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

	// AI (OpenAI only)
	OpenAIAPIKey       string
	OpenAIModel        string // scraping + classification: gpt-4o-mini (cheap, fast)
	OpenAICommentModel string // comments + inbox generation: gpt-4o (high quality)
	AgentBrainURL      string // optional Python sidecar planner endpoint base URL
	AgentBrainTimeout  int    // milliseconds

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

	// noVNC browser workspace
	VNCPort    int // VNC server TCP port (default 5900)
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
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	cfg := &Config{
		TelegramBotToken:   getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramAdminChat:  getEnvInt64("TELEGRAM_ADMIN_CHAT_ID", 0),
		TelegramOrgID:      getEnvInt64("TELEGRAM_ORG_ID", 1),
		OpenAIAPIKey:       getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:        getEnv("OPENAI_MODEL", "gpt-4o-mini"),
		OpenAICommentModel: getEnv("OPENAI_COMMENT_MODEL", "gpt-4.1"),
		AgentBrainURL:      getEnv("AGENT_BRAIN_URL", ""),
		AgentBrainTimeout:  getEnvInt("AGENT_BRAIN_TIMEOUT_MS", 1500),
		APISecret:          getEnv("API_SECRET", ""),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		EncryptionKey:      getEnv("ENCRYPTION_KEY", ""),
		AllowedOrigins:     getEnv("ALLOWED_ORIGINS", ""),
		AdminEmail:         getEnv("ADMIN_EMAIL", ""),
		AdminPassword:      getEnv("ADMIN_PASSWORD", ""),
		AdminName:          getEnv("ADMIN_NAME", "Admin"),
		ChromePath:         getEnv("CHROME_PATH", ""),
		ProfileDir:         getEnv("PROFILE_DIR", "data/profiles"),
		Headless:           detectHeadless(),
		ServerHost:         getEnv("SERVER_HOST", ""),
		SSHPort:            getEnvInt("SSH_PORT", 22),
		VNCPort:            getEnvInt("VNC_PORT", 5900),
		CDPPort:            getEnvInt("CDP_PORT", 9222),
		DisplayNum:         getEnvInt("DISPLAY_NUM", 99),
		WebPort:            getEnvInt("WEB_PORT", 8080),
		DBPath:             getEnv("DB_PATH", "data/scraper.db"),
		BackupEnabled:      getEnv("BACKUP_ENABLED", "true") == "true",
		MaxWorkers:         getEnvInt("MAX_WORKERS", 1),
		ScrollTimeout:      getEnvInt("SCROLL_TIMEOUT_SEC", 60),
		ScanIntervalMin:    getEnvInt("SCAN_INTERVAL_MIN", 30),
		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURI:  getEnv("GOOGLE_REDIRECT_URI", ""),
		SMTPHost:           getEnv("SMTP_HOST", ""),
		SMTPPort:           getEnvInt("SMTP_PORT", 587),
		SMTPUsername:       getEnv("SMTP_USERNAME", ""),
		SMTPPassword:       getEnv("SMTP_PASSWORD", ""),
		SMTPFromEmail:      getEnv("SMTP_FROM_EMAIL", ""),
		SMTPFromName:       getEnv("SMTP_FROM_NAME", "THG AutoFlow"),
		SMTPTLS:            getEnvBool("SMTP_TLS", false),
		SMTPStartTLS:       getEnvBool("SMTP_STARTTLS", true),
		SMTPSkipVerify:     getEnvBool("SMTP_SKIP_VERIFY", false),
		AppBaseURL:         getEnv("APP_BASE_URL", getEnv("PUBLIC_APP_URL", getEnv("NEXT_PUBLIC_SITE_URL", ""))),
	}

	if proxyStr := getEnv("PROXY_LIST", ""); proxyStr != "" {
		cfg.ProxyList = strings.Split(proxyStr, ",")
		for i := range cfg.ProxyList {
			cfg.ProxyList[i] = strings.TrimSpace(cfg.ProxyList[i])
		}
	}

	return cfg
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
