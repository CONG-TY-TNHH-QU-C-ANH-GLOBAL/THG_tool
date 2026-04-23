package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Telegram
	TelegramBotToken  string
	TelegramAdminChat int64

	// AI (OpenAI only)
	OpenAIAPIKey       string
	OpenAIModel        string // scraping + classification: gpt-4o-mini (cheap, fast)
	OpenAICommentModel string // comments + inbox generation: gpt-4o (high quality)

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
		OpenAIAPIKey:       getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:        getEnv("OPENAI_MODEL", "gpt-4o-mini"),
		OpenAICommentModel: getEnv("OPENAI_COMMENT_MODEL", "gpt-4.1"),
		APISecret:          getEnv("API_SECRET", ""),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		EncryptionKey:      getEnv("ENCRYPTION_KEY", ""),
		AllowedOrigins:     getEnv("ALLOWED_ORIGINS", ""),
		AdminEmail:         getEnv("ADMIN_EMAIL", ""),
		AdminPassword:      getEnv("ADMIN_PASSWORD", ""),
		AdminName:          getEnv("ADMIN_NAME", "Admin"),
		ChromePath:         getEnv("CHROME_PATH", ""),
		ProfileDir:         getEnv("PROFILE_DIR", "data/profiles"),
		WebPort:            getEnvInt("WEB_PORT", 8080),
		DBPath:             getEnv("DB_PATH", "data/scraper.db"),
		BackupEnabled:      getEnv("BACKUP_ENABLED", "true") == "true",
		MaxWorkers:         getEnvInt("MAX_WORKERS", 1),
		ScrollTimeout:      getEnvInt("SCROLL_TIMEOUT_SEC", 60),
		ScanIntervalMin:    getEnvInt("SCAN_INTERVAL_MIN", 30),
	}

	if proxyStr := getEnv("PROXY_LIST", ""); proxyStr != "" {
		cfg.ProxyList = strings.Split(proxyStr, ",")
		for i := range cfg.ProxyList {
			cfg.ProxyList[i] = strings.TrimSpace(cfg.ProxyList[i])
		}
	}

	return cfg
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
