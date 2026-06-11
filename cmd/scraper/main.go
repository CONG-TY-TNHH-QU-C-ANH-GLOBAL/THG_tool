package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/thg/scraper/internal/ai"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/browser"
	"github.com/thg/scraper/internal/config"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/logstream"
	"github.com/thg/scraper/internal/mailer"
	"github.com/thg/scraper/internal/server"
	session_pkg "github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/skills"
	"github.com/thg/scraper/internal/store"
	tgclient "github.com/thg/scraper/internal/telegram/client"
	"github.com/thg/scraper/internal/workspace"
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

	// Production refuses to boot when JWT/encryption secrets are missing —
	// otherwise we silently store Facebook cookies unencrypted or run with
	// API auth disabled. Set APP_ENV=production to enable the strict check.
	if err := cfg.MustValidateProductionSecrets(); err != nil {
		log.Fatalf("❌ %v", err)
	}
	if cfg.JWTSecret == "" {
		log.Println("⚠️  JWT_SECRET not set — API authentication is DISABLED. Set it in production (APP_ENV=production blocks startup).")
	}
	if cfg.EncryptionKey == "" {
		log.Println("⚠️  ENCRYPTION_KEY not set — Facebook cookies stored unencrypted. Set it in production (APP_ENV=production blocks startup).")
	}

	// Initialize database
	db, err := store.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("❌ Database init failed: %v", err)
	}
	defer db.Close()
	db.SetEncryptionKey(cfg.EncryptionKey)
	log.Println("✅ Database initialized")

	// Bootstrap first admin user if ADMIN_EMAIL + ADMIN_PASSWORD are set and no users exist
	if cfg.AdminEmail != "" && cfg.AdminPassword != "" {
		hash, err := authpkg.HashPassword(cfg.AdminPassword)
		if err != nil {
			log.Fatalf("❌ Admin password hashing failed: %v", err)
		}
		if err := db.EnsureAdminUser(cfg.AdminEmail, hash, cfg.AdminName); err != nil {
			log.Printf("⚠️  Admin bootstrap failed: %v", err)
		} else {
			log.Printf("✅ Admin user ready: %s", cfg.AdminEmail)
		}
	}

	// Upsert superadmin unconditionally — works even when DB already has users.
	// Set SUPERADMIN_EMAIL + SUPERADMIN_PASSWORD in .env to activate.
	if saEmail := os.Getenv("SUPERADMIN_EMAIL"); saEmail != "" {
		saPass := os.Getenv("SUPERADMIN_PASSWORD")
		if saPass == "" {
			log.Println("⚠️  SUPERADMIN_EMAIL set but SUPERADMIN_PASSWORD is empty — skipping")
		} else {
			hash, err := authpkg.HashPassword(saPass)
			if err != nil {
				log.Printf("⚠️  Superadmin password hashing failed: %v", err)
			} else if err := db.EnsureFounder(saEmail, hash, os.Getenv("SUPERADMIN_NAME")); err != nil {
				log.Printf("⚠️  Superadmin upsert failed: %v", err)
			} else {
				log.Printf("✅ Superadmin ready: %s", saEmail)
			}
		}
	}

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
	// Store.ResetStaleExecutingForOrg, called during normal runtime
	// (not at startup).

	// Initialize job store (scheduler_jobs table — idempotent, replaces chan-based queue)
	jobStore, err := jobs.NewStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("❌ Job store init failed: %v", err)
	}
	log.Println("✅ Job store initialized")

	// Initialize AppStore (app_tasks, task_leads, browser infra tables)
	appStore, err := store.NewAppStore(db)
	if err != nil {
		log.Fatalf("❌ AppStore init failed: %v", err)
	}
	log.Println("✅ AppStore initialized")

	// Initialize workspace manager (per-account live Chrome for dashboard browser view)
	workspaceMgr := workspace.NewManager(cfg.ChromePath, cfg.ProfileDir)

	// Wire persistent PortRegistry so containers get deterministic host ports
	portRegistry := workspace.NewPortRegistry(appStore.DB())
	if err := portRegistry.LoadFromDB(ctx); err != nil {
		log.Printf("⚠️  PortRegistry DB load failed: %v", err)
	}
	portRegistry.ReconcileFromDocker()
	workspaceMgr.SetPortRegistry(portRegistry)

	workspaceMgr.ReconcileRunning() // re-attach containers that survived a server restart
	if os.Getenv("WORKSPACE_STOP_ON_SHUTDOWN") == "1" {
		defer workspaceMgr.StopAll()
	} else {
		log.Println("[Workspace] Browser containers will survive API shutdown for session continuity")
	}
	log.Println("✅ Workspace manager initialized")

	// Circuit breaker + health checker — prevent restart storms
	cb := workspace.NewCircuitBreaker(appStore.DB(), func(msg string) {
		log.Printf("[CircuitBreaker] ALERT: %s", msg)
	})
	restartCtrl := workspace.NewRestartController(workspaceMgr, cb)
	healthChecker := workspace.NewHealthChecker()
	go healthChecker.Run(ctx, workspaceMgr, func(accountID int64) {
		restartCtrl.OnUnhealthy(ctx, accountID)
	})
	log.Println("✅ Health checker started (15s interval)")

	// Keep login/checkpoint sessions untouched unless ops explicitly enables
	// the watchdog. HealthChecker still keeps the container observable via VNC.
	if os.Getenv("WORKSPACE_SESSION_WATCHDOG") == "1" {
		watchdog := browser.NewWatchdog(workspaceMgr, 30*time.Second, func(accountID int64, outcome browser.SessionOutcome, reason string) {
			switch outcome {
			case browser.SessionCDPDown:
				if os.Getenv("WORKSPACE_AUTO_RESTART_CDP_DOWN") != "1" {
					log.Printf("[Watchdog] CDP_DOWN account %d - keeping browser alive during login/session flow: %s", accountID, reason)
					return
				}
				log.Printf("[Watchdog] CDP_DOWN account %d — safe restart: %s", accountID, reason)
				if err := browser.SafeRestart(ctx, workspaceMgr, accountID, ""); err != nil {
					log.Printf("[Watchdog] SafeRestart failed account %d: %v", accountID, err)
				}
			case browser.SessionCheckpoint:
				log.Printf("[Watchdog] CHECKPOINT account %d — manual login required: %s", accountID, reason)
			case browser.SessionExpired:
				log.Printf("[Watchdog] EXPIRED account %d — session lost: %s", accountID, reason)
			case browser.SessionBlocked:
				log.Printf("[Watchdog] BLOCKED account %d — ban detected: %s", accountID, reason)
			}
		})
		go watchdog.Run(ctx)
		log.Println("✅ Session watchdog started (30s interval)")
	} else {
		log.Println("[Workspace] Session watchdog disabled; browser login runs VNC-only until manual sync")
	}

	// Session registry — in-memory mirror for fast API reads
	sessionReg := session_pkg.NewRegistry(appStore)
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
	var telegramNotify func(string)
	var agent *ai.Agent
	var classifierMg, commentMg *ai.MessageGenerator
	skillRegistry := skills.NewRegistry()
	if cfg.OpenAIAPIKey != "" {
		classifierMg = ai.NewMessageGenerator(cfg.OpenAIAPIKey, cfg.OpenAIClassifierModel)
		commentMg = ai.NewMessageGenerator(cfg.OpenAIAPIKey, cfg.OpenAICommentModel)
		agent = ai.NewAgent(cfg.OpenAIAPIKey, cfg.OpenAICommentModel, db)
		if cfg.AgentBrainURL != "" {
			agent.SetBrainClient(ai.NewBrainClient(cfg.AgentBrainURL, time.Duration(cfg.AgentBrainTimeout)*time.Millisecond))
			log.Printf("✅ Agent Brain sidecar enabled: %s", cfg.AgentBrainURL)
		}
		actionHandler := makeAgentActionHandler(db, jobStore, commentMg, func(msg string) {
			if telegramNotify != nil {
				telegramNotify(msg)
			}
		})
		agent.ActionHandler = actionHandler

		// Phase 6: register the open-prompt skill catalog. Each skill
		// captures the action handler so its Run closure can re-route
		// into the existing production logic without duplicating it.
		registerBuiltinSkills(skillRegistry, builtinSkillDeps{
			db:       db,
			jobStore: jobStore,
			msgGen:   commentMg,
			notify: func(msg string) {
				if telegramNotify != nil {
					telegramNotify(msg)
				}
			},
			handler: actionHandler,
		})
		agent.SetSkillRegistry(skillRegistry)
		log.Printf("✅ AI Agent initialized (classifier: %s, comment: %s, skills=%d)",
			cfg.OpenAIClassifierModel, cfg.OpenAICommentModel, len(skillRegistry.All()))
	} else {
		log.Println("⚠️  OPENAI_API_KEY not set, AI Agent disabled")
	}

	// Telegram (optional). The legacy single-org long-poll agent-bot was RETIRED (see
	// specs/TELEGRAM_BOT_RUNTIME.md): long-poll (getUpdates) and a webhook cannot share one bot
	// token, and the product direction is a tenant-scoped integration, not a single-org side bot.
	// The tenant-scoped webhook control-plane (POST /api/telegram/webhook, wired in the server) is
	// now the SINGLE Telegram runtime. Here we only wire the system notifier to send to the
	// configured admin chat via the thin Telegram client.
	if cfg.TelegramBotToken != "" {
		tgClient := tgclient.New(cfg.TelegramBotToken)
		if cfg.TelegramAdminChat != 0 {
			telegramNotify = func(msg string) { _ = tgClient.Send(cfg.TelegramAdminChat, msg) }
		}
		log.Println("✅ Telegram client initialized (webhook runtime; legacy long-poll bot retired)")
	} else {
		log.Println("⚠️  Telegram bot token not set, Telegram disabled")
	}

	go runCrawlIntentScheduler(ctx, db, jobStore, time.Minute)
	log.Println("✅ Recurring crawl intent scheduler started (org plans → 30m+ automation)")

	go runAutoArchiveScheduler(ctx, db, cfg)
	log.Println("✅ Auto-archive scheduler started (lead lifecycle retention)")

	go runCommentReverifyScheduler(ctx, db, cfg)
	log.Println("✅ Comment reverify scheduler started (submitted_unverified → reverify queue)")

	// Start web server (non-blocking)
	srv := server.New(db, jobStore, agent, workspaceMgr, server.Config{
		Port:                   cfg.WebPort,
		JWTSecret:              cfg.JWTSecret,
		AllowedOrigins:         cfg.AllowedOrigins,
		ChromePath:             cfg.ChromePath,
		ProfileDir:             cfg.ProfileDir,
		Headless:               cfg.Headless,
		ServerHost:             cfg.ServerHost,
		SSHPort:                cfg.SSHPort,
		GoogleClientID:         cfg.GoogleClientID,
		GoogleClientSecret:     cfg.GoogleClientSecret,
		GoogleRedirectURI:      cfg.GoogleRedirectURI,
		TelegramBotToken:       cfg.TelegramBotToken,
		TelegramBotEnabled:     cfg.TelegramBotEnabled,
		TelegramNotifyEnabled:  cfg.TelegramNotifyEnabled,
		TelegramActionsEnabled: cfg.TelegramActionsEnabled,
		TelegramWebhookSecret:  cfg.TelegramWebhookSecret,
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
		Notifier: func(msg string) {
			if telegramNotify != nil {
				telegramNotify(msg)
			}
		},
	})

	srv.SetSessionRegistry(sessionReg)
	if classifierMg != nil {
		// Reclassify endpoint + crawl-result handler call UniversalClassify,
		// which is high-volume and schema-locked → use the cheap classifier
		// model, not the strong comment model.
		srv.SetUniversalClassifier(classifierMg)
	}

	go func() {
		if err := srv.Start(); err != nil {
			log.Printf("⚠️  Web server error: %v", err)
		}
	}()
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
