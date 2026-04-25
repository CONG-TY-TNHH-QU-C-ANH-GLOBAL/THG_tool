package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/thg/scraper/internal/accounts"
	"github.com/thg/scraper/internal/ai"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/browser"
	"github.com/thg/scraper/internal/config"
	"github.com/thg/scraper/internal/logstream"
	"github.com/thg/scraper/internal/orchestrator"
	"github.com/thg/scraper/internal/queue"
	"github.com/thg/scraper/internal/server"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram"
	"github.com/thg/scraper/internal/workspace"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	logstream.Install() // capture all log.Printf output for the Logs dashboard page
	log.Println("🕷️  THG Agentic Scraper v2 — Starting...")

	// Load .env file (optional)
	if err := godotenv.Load(); err != nil {
		log.Printf("ℹ️  Note: .env file not found or could not be loaded: %v", err)
	} else {
		log.Println("✅ .env file loaded successfully")
	}

	// Load configuration
	cfg := config.Load()

	// Warn on missing production secrets
	if cfg.JWTSecret == "" {
		log.Println("⚠️  JWT_SECRET not set — API authentication is DISABLED. Set it in production!")
	}
	if cfg.EncryptionKey == "" {
		log.Println("⚠️  ENCRYPTION_KEY not set — Facebook cookies stored unencrypted. Set it in production!")
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

	// Auto-backup SQLite daily (Fix #4: data protection)
	if cfg.BackupEnabled {
		db.StartAutoBackup(cfg.DBPath)
	}

	// Security warning: Chrome profiles
	log.Printf("🔒 Chrome profiles at: %s (contains FB session — NEVER commit to git!)", cfg.ProfileDir)

	// Reset any orphaned approved outbound messages from previous crashes/restarts
	if err := db.ResetOrphanedOutbounds(); err != nil {
		log.Printf("⚠️ Failed to reset orphaned outbounds: %v", err)
	}

	// Initialize browser pool (with persistent profile support)
	proxyURL := ""
	if len(cfg.ProxyList) > 0 {
		proxyURL = cfg.ProxyList[0]
	}
	pool, err := browser.NewPool(cfg.MaxWorkers, cfg.ChromePath, proxyURL, cfg.ProfileDir)
	if err != nil {
		log.Printf("⚠️  Browser pool init failed: %v (scraping disabled)", err)
		pool = nil
	} else {
		defer pool.Shutdown()
		log.Printf("✅ Browser pool initialized (%d contexts, profile: %s)", cfg.MaxWorkers, cfg.ProfileDir)
	}

	// Initialize job queue
	q := queue.New(db, cfg.MaxWorkers)

	// Initialize AI classifier (OpenAI)
	classifier := ai.NewClassifier(cfg.OpenAIAPIKey, cfg.OpenAIModel, db)

	// Initialize AI message generator — uses gpt-4o for high-quality comments + inbox
	var msgGen *ai.MessageGenerator
	if cfg.OpenAIAPIKey != "" {
		msgGen = ai.NewMessageGenerator(cfg.OpenAIAPIKey, cfg.OpenAICommentModel)
		log.Printf("✅ AI MessageGenerator initialized (model: %s)", cfg.OpenAICommentModel)
	}

	// Initialize account manager (for multi-account Facebook access)
	accountMgr := accounts.NewManager(db, cfg.ChromePath, cfg.ProfileDir)
	log.Printf("✅ Account manager initialized (profiles: %s)", cfg.ProfileDir)

	// Initialize workspace manager (per-account live Chrome for dashboard browser view)
	workspaceMgr := workspace.NewManager(cfg.ChromePath, cfg.ProfileDir)
	defer workspaceMgr.StopAll()
	log.Println("✅ Workspace manager initialized")

	// Initialize price extractor (OpenAI Vision for reading price list images)
	var pricer *ai.PriceExtractor
	if cfg.OpenAIAPIKey != "" {
		pricer = ai.NewPriceExtractor(cfg.OpenAIAPIKey, cfg.OpenAIModel)
		log.Println("✅ Price extractor initialized")
	}

	// Initialize AI Agent (OpenAI Function Calling) — v2
	var agent *ai.Agent
	if cfg.OpenAIAPIKey != "" {
		agent = ai.NewAgent(cfg.OpenAIAPIKey, cfg.OpenAICommentModel, db)
		log.Printf("✅ AI Agent initialized (model: %s)", cfg.OpenAICommentModel)
	} else {
		log.Println("⚠️  OPENAI_API_KEY not set, AI Agent disabled")
	}

	// Initialize Telegram bot (optional)
	var bot *telegram.Bot
	if cfg.TelegramBotToken != "" {
		bot, err = telegram.New(cfg.TelegramBotToken, cfg.TelegramAdminChat, db, q, agent, pricer)
		if err != nil {
			log.Printf("⚠️  Telegram bot init failed: %v", err)
		} else {
			log.Println("✅ Telegram bot initialized")
		}
	} else {
		log.Println("⚠️  Telegram bot token not set, bot disabled")
	}

	// Initialize orchestrator
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Wrap *browser.Pool in the interface (nil pool must stay nil-interface, not nil-pointer-in-interface)
	var poolBrowser browser.Browser
	if pool != nil {
		poolBrowser = pool
	}
	orch := orchestrator.New(db, poolBrowser, q, bot, classifier, msgGen, accountMgr, pricer)
	scanInterval := time.Duration(cfg.ScanIntervalMin) * time.Minute
	orch.Start(ctx, scanInterval)
	defer orch.Stop()
	log.Printf("✅ Orchestrator started (scan every %d min)", cfg.ScanIntervalMin)

	// Wire AI Agent action handler to orchestrator
	if agent != nil {
		agent.ActionHandler = orch.HandleAgentAction
	}

	// Start Telegram bot (non-blocking)
	if bot != nil {
		go bot.Start()
		defer bot.Stop()
	}

	// Start web server (non-blocking)
	srv := server.New(db, q, agent, workspaceMgr, server.Config{
		Port:           cfg.WebPort,
		JWTSecret:      cfg.JWTSecret,
		AllowedOrigins: cfg.AllowedOrigins,
		ChromePath:     cfg.ChromePath,
		ProfileDir:     cfg.ProfileDir,
		Headless:       cfg.Headless,
		ServerHost:     cfg.ServerHost,
		SSHPort:        cfg.SSHPort,
		VNCPort:        cfg.VNCPort,
		CDPPort:        cfg.CDPPort,
		DisplayNum:     cfg.DisplayNum,
	})
	// Wire agent post-processor so scraped posts from local agents run through AI pipeline
	srv.SetPostProcessor(orch.ProcessAgentScrapedPosts)

	go func() {
		if err := srv.Start(); err != nil {
			log.Printf("⚠️  Web server error: %v", err)
		}
	}()
	defer srv.Shutdown()

	log.Printf("🚀 System ready! Web UI: http://localhost:%d", cfg.WebPort)
	if bot != nil {
		log.Println("🤖 Telegram bot is listening for commands")
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("🛑 Shutting down gracefully...")
}
