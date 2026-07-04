package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/thg/scraper/internal/browser"
	"github.com/thg/scraper/internal/config"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/logstream"
	"github.com/thg/scraper/internal/mailer"
	"github.com/thg/scraper/internal/server"
	session_pkg "github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	logstream.Install() // capture all log.Printf output for the Logs dashboard page
	log.Println("THG AutoFlow Agent Workspace — Starting...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load .env file (optional)
	if err := godotenv.Load(); err != nil {
		log.Printf("ℹ️  Note: .env file not found or could not be loaded: %v", err)
	} else {
		log.Println("✅ .env file loaded successfully")
	}

	// Load configuration
	cfg := config.Load()

	validateProductionSecrets(cfg)

	// Initialize database
	db, err := store.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("❌ Database init failed: %v", err)
	}
	defer db.Close()
	db.SetEncryptionKey(cfg.EncryptionKey)
	log.Println("✅ Database initialized")

	bootstrapAdminUser(cfg, db)
	bootstrapSuperadmin(db)

	// Auto-backup SQLite daily (Fix #4: data protection)
	if cfg.BackupEnabled {
		db.StartAutoBackup(cfg.DBPath)
	}

	// Security warning: Chrome profiles
	log.Printf("🔒 Chrome profiles at: %s (contains FB session — NEVER commit to git!)", cfg.ProfileDir)

	// PR-2 (V2 staged refactor): the legacy ResetOrphanedOutbounds startup
	// hook was removed. In the autonomous-first model, planned rows must
	// RESUME after a restart, not be marked failed. Stale executing rows
	// are reclaimed per-org via the lease mechanism in
	// outbound.Store.ResetStaleExecuting, called during normal runtime
	// (not at startup).

	// Initialize job store (scheduler_jobs table — idempotent, replaces chan-based queue)
	jobStore, err := jobs.NewStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("❌ Job store init failed: %v", err)
	}
	log.Println("✅ Job store initialized")

	// Initialize workspace manager (per-account live Chrome for dashboard browser view)
	// (app_tasks/task_leads/browser-infra bootstrap now runs inside store.New — app.Migrate)
	workspaceMgr := initPortRegistry(ctx, cfg, db.DB())

	workspaceMgr.ReconcileRunning() // re-attach containers that survived a server restart
	if os.Getenv("WORKSPACE_STOP_ON_SHUTDOWN") == "1" {
		defer workspaceMgr.StopAll()
	} else {
		log.Println("[Workspace] Browser containers will survive API shutdown for session continuity")
	}
	log.Println("✅ Workspace manager initialized")

	// Circuit breaker + health checker — prevent restart storms
	startHealthMonitoring(ctx, workspaceMgr, db.DB())

	// Keep login/checkpoint sessions untouched unless ops explicitly enables
	// the watchdog. HealthChecker still keeps the container observable via VNC.
	if os.Getenv("WORKSPACE_SESSION_WATCHDOG") == "1" {
		watchdog := browser.NewWatchdog(workspaceMgr, 30*time.Second, watchdogOutcomeHandler(ctx, workspaceMgr))
		go watchdog.Run(ctx)
		log.Println("✅ Session watchdog started (30s interval)")
	} else {
		log.Println("[Workspace] Session watchdog disabled; browser login runs VNC-only until manual sync")
	}

	// Session registry — in-memory mirror for fast API reads
	sessionReg := session_pkg.NewRegistry(db.Sessions())
	if err := sessionReg.LoadAll(ctx); err != nil {
		log.Printf("⚠️  Session registry load failed: %v", err)
	}

	// Initialize price extractor (OpenAI Vision for reading price list images).

	// Initialize AI Agent (OpenAI Function Calling) — v2.
	//
	// Two MessageGenerator instances on purpose:
	//   - classifierMg: high-volume, schema-locked classification (UniversalClassify).
	//     Cheap+fast model (OPENAI_CLASSIFIER_MODEL).
	//   - commentMg: user-facing comment/inbox/post generation.
	//     Strong model (OPENAI_COMMENT_MODEL).
	// Both share the same API key + http.Client; the only difference is the
	// model field. Splitting the two avoids paying for the strong model on
	// every classified post.
	// telegramNotify is assigned below (once the Telegram client is known);
	// notify closes over it by reference so it can be handed to callbacks
	// that are constructed before telegramNotify's value is set.
	var telegramNotify func(string)
	notify := func(msg string) {
		if telegramNotify != nil {
			telegramNotify(msg)
		}
	}
	agent, classifierMg, commentMg := setupAIAgent(cfg, db, jobStore, notify)

	// Telegram (optional). The legacy single-org long-poll agent-bot was RETIRED (see
	// specs/TELEGRAM_BOT_RUNTIME.md): long-poll (getUpdates) and a webhook cannot share one bot
	// token, and the product direction is a tenant-scoped integration, not a single-org side bot.
	// The tenant-scoped webhook control-plane (POST /api/telegram/webhook, wired in the server) is
	// now the SINGLE Telegram runtime. Here we only wire the system notifier to send to the
	// configured admin chat via the thin Telegram client.
	telegramNotify = setupTelegramNotifier(cfg)

	go runCrawlIntentScheduler(ctx, db, jobStore, time.Minute)
	log.Println("✅ Recurring crawl intent scheduler started (org plans → 30m+ automation)")

	go runAutoArchiveScheduler(ctx, db, cfg)
	log.Println("✅ Auto-archive scheduler started (lead lifecycle retention)")

	go runCommentReverifyScheduler(ctx, db, cfg)
	log.Println("✅ Comment reverify scheduler started (submitted_unverified → reverify queue)")

	// P1 PR-2: direct-post intake process manager — observes the imported post lead
	// and queues the comment durably (no-op when no comment generator is configured).
	go runDirectPostIntakeScheduler(ctx, db, commentMg, notify)
	log.Println("✅ Direct-post intake scheduler started (unknown post → import → auto-comment)")

	// Start web server (non-blocking)
	srv := server.New(db, jobStore, agent, workspaceMgr, server.Config{
		Port:                        cfg.WebPort,
		JWTSecret:                   cfg.JWTSecret,
		AllowedOrigins:              cfg.AllowedOrigins,
		ChromePath:                  cfg.ChromePath,
		ProfileDir:                  cfg.ProfileDir,
		Headless:                    cfg.Headless,
		ServerHost:                  cfg.ServerHost,
		SSHPort:                     cfg.SSHPort,
		GoogleClientID:              cfg.GoogleClientID,
		GoogleClientSecret:          cfg.GoogleClientSecret,
		GoogleRedirectURI:           cfg.GoogleRedirectURI,
		TelegramBotToken:            cfg.TelegramBotToken,
		TelegramBotEnabled:          cfg.TelegramBotEnabled,
		TelegramNotifyEnabled:       cfg.TelegramNotifyEnabled,
		TelegramActionsEnabled:      cfg.TelegramActionsEnabled,
		TelegramWebhookSecret:       cfg.TelegramWebhookSecret,
		TelegramAllowGlobalFallback: cfg.TelegramAllowGlobalFallback,
		Mailer: mailer.Config{
			Host:               cfg.SMTPHost,
			Port:               cfg.SMTPPort,
			Username:           cfg.SMTPUsername,
			Password:           cfg.SMTPPassword,
			FromEmail:          cfg.SMTPFromEmail,
			FromName:           cfg.SMTPFromName,
			AppBaseURL:         cfg.AppBaseURL,
			UseTLS:             cfg.SMTPTLS,
			UseStartTLS:        cfg.SMTPStartTLS,
			InsecureSkipVerify: cfg.SMTPSkipVerify,
			Timeout:            10 * time.Second,
		},
		Notifier: notify,
	})

	srv.SetSessionRegistry(sessionReg)
	if classifierMg != nil {
		// Reclassify endpoint + crawl-result handler call UniversalClassify,
		// which is high-volume and schema-locked → use the cheap classifier
		// model, not the strong comment model.
		srv.SetUniversalClassifier(classifierMg)
	}

	go serveHTTP(srv)
	defer srv.Shutdown()

	log.Printf("🚀 System ready! Web UI: http://localhost:%d", cfg.WebPort)
	if cfg.TelegramBotToken != "" {
		log.Println("🤖 Telegram webhook runtime ready at POST /api/telegram/webhook")
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("🛑 Shutting down gracefully...")
	cancel() // stop health checker and other ctx-bound goroutines
}
