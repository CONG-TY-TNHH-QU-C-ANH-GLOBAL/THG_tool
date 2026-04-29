package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/thg/scraper/internal/ai"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/browser"
	"github.com/thg/scraper/internal/config"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/logstream"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server"
	session_pkg "github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram"
	"github.com/thg/scraper/internal/workspace"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	logstream.Install() // capture all log.Printf output for the Logs dashboard page
	log.Println("🕷️  THG Agentic Scraper v2 — Starting...")

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
			} else if err := db.EnsureSuperAdmin(saEmail, hash, os.Getenv("SUPERADMIN_NAME")); err != nil {
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

	// Reset any orphaned approved outbound messages from previous crashes/restarts
	if err := db.ResetOrphanedOutbounds(); err != nil {
		log.Printf("⚠️ Failed to reset orphaned outbounds: %v", err)
	}

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
	defer workspaceMgr.StopAll()
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

	watchdog := browser.NewWatchdog(workspaceMgr, 30*time.Second, func(accountID int64, outcome browser.SessionOutcome, reason string) {
		switch outcome {
		case browser.SessionCDPDown:
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

	// Session registry — in-memory mirror for fast API reads
	sessionReg := session_pkg.NewRegistry(appStore)
	if err := sessionReg.LoadAll(ctx); err != nil {
		log.Printf("⚠️  Session registry load failed: %v", err)
	}

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
		agent.ActionHandler = makeAgentActionHandler(db, jobStore)
		log.Printf("✅ AI Agent initialized (model: %s)", cfg.OpenAICommentModel)
	} else {
		log.Println("⚠️  OPENAI_API_KEY not set, AI Agent disabled")
	}

	// Initialize Telegram bot (optional)
	var bot *telegram.Bot
	if cfg.TelegramBotToken != "" {
		bot, err = telegram.New(cfg.TelegramBotToken, cfg.TelegramAdminChat, db, jobStore, agent, pricer)
		if err != nil {
			log.Printf("⚠️  Telegram bot init failed: %v", err)
		} else {
			log.Println("✅ Telegram bot initialized")
		}
	} else {
		log.Println("⚠️  Telegram bot token not set, bot disabled")
	}

	// Start Telegram bot (non-blocking)
	if bot != nil {
		go bot.Start()
		defer bot.Stop()
	}

	// Start web server (non-blocking)
	srv := server.New(db, jobStore, agent, workspaceMgr, server.Config{
		Port:               cfg.WebPort,
		JWTSecret:          cfg.JWTSecret,
		AllowedOrigins:     cfg.AllowedOrigins,
		ChromePath:         cfg.ChromePath,
		ProfileDir:         cfg.ProfileDir,
		Headless:           cfg.Headless,
		ServerHost:         cfg.ServerHost,
		SSHPort:            cfg.SSHPort,
		GoogleClientID:     cfg.GoogleClientID,
		GoogleClientSecret: cfg.GoogleClientSecret,
		GoogleRedirectURI:  cfg.GoogleRedirectURI,
	})

	srv.SetSessionRegistry(sessionReg)

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
	cancel() // stop health checker and other ctx-bound goroutines
}

func makeAgentActionHandler(db *store.Store, jobStore *jobs.Store) func(string, map[string]any) (string, error) {
	return func(action string, args map[string]any) (string, error) {
		switch action {
		case "set_context":
			key, value := argString(args, "key"), argString(args, "value")
			if key == "" || value == "" {
				return "", fmt.Errorf("set_context requires key and value")
			}
			if err := db.SetContext(key, value); err != nil {
				return "", err
			}
			return fmt.Sprintf("da luu context %q", key), nil
		case "describe_business":
			desc := argString(args, "description")
			if desc == "" {
				return "", fmt.Errorf("describe_business requires description")
			}
			if err := db.SetContext("business_desc", desc); err != nil {
				return "", err
			}
			return "da luu mo ta doanh nghiep cho crawler/classifier", nil
		case "get_stats":
			stats, err := db.GetStats()
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("posts=%d leads=%d hot=%d jobs_running=%d", stats.TotalPosts, stats.TotalLeads, stats.HotLeads, stats.RunningJobs), nil
		case "add_group":
			u, name := argString(args, "url"), argString(args, "name")
			if u == "" {
				return "", fmt.Errorf("add_group requires url")
			}
			if name == "" {
				name = u
			}
			id, err := db.AddGroup(&models.Group{
				Platform:  detectPlatformFromURL(u),
				Name:      name,
				URL:       u,
				Active:    true,
				JoinState: "none",
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("da them group #%d", id), nil
		case "scrape_group":
			u := argString(args, "url")
			if u == "" {
				return "", fmt.Errorf("scrape_group requires url")
			}
			return submitOpenCrawl(context.Background(), jobStore, "facebook_crawl", []jobs.Source{{Type: sourceTypeFromURL(u), URL: u, Label: "prompt_url"}}, args)
		case "scrape_comments":
			u := argString(args, "post_url")
			if u == "" {
				return "", fmt.Errorf("scrape_comments requires post_url")
			}
			return submitOpenCrawl(context.Background(), jobStore, "facebook_crawl", []jobs.Source{{Type: "facebook_post", URL: u, Label: "prompt_post"}}, args)
		case "scrape_all":
			return "", fmt.Errorf("scrape_all fixed configured groups is disabled in production; ask for a target URL or search query")
		case "classify_leads":
			return "classification runs inline during every crawler job using the current business context", nil
		default:
			return "", fmt.Errorf("agent action %q is not wired to a production handler yet", action)
		}
	}
}

func submitOpenCrawl(ctx context.Context, jobStore *jobs.Store, intent string, sources []jobs.Source, args map[string]any) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("crawler requires at least one source")
	}
	maxItems := int(argInt64(args, "max_items"))
	if maxItems <= 0 {
		maxItems = 50
	}
	keywords := splitKeywords(argString(args, "keywords"))
	task := &jobs.Task{
		SchemaVersion: "1",
		TaskID:        openCrawlTaskID(intent, sources, args),
		AccountID:     argInt64(args, "account_id"),
		Intent:        intent,
		Keywords:      keywords,
		CrawlPlan:     jobs.CrawlPlan{Sources: sources, MaxItems: maxItems, BatchSize: 20},
		Filters:       jobs.Filters{Keywords: keywords, MinContentLength: 20, KeywordMinScore: 0},
		ScoringConfig: jobs.ScoringConfig{
			HotThreshold:  70,
			WarmThreshold: 40,
			Weights: jobs.ScoringWeights{
				KeywordRelevance: 0.4,
				Engagement:       0.2,
				ContentQuality:   0.4,
			},
		},
		RetryPolicy:         jobs.RetryPolicy{MaxAttempts: 3, BackoffMs: 1000},
		ExecutionMode:       "async",
		OutputSchema:        "open_crawler_v1",
		OutputSchemaVersion: "1",
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return "", err
	}
	job, err := jobStore.Submit(ctx, task, string(payload))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("da tao crawler job #%d task=%s intent=%s", job.ID, job.TaskID, intent), nil
}

func openCrawlTaskID(intent string, sources []jobs.Source, args map[string]any) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|day=%s|", intent, time.Now().UTC().Format("2006-01-02"))
	for _, src := range sources {
		fmt.Fprintf(h, "%s:%s|", src.Type, src.URL)
	}
	fmt.Fprintf(h, "account=%d", argInt64(args, "account_id"))
	return fmt.Sprintf("open-crawl-%x", h.Sum(nil))[:27]
}

func argString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case fmt.Stringer:
		return strings.TrimSpace(t.String())
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func argInt64(args map[string]any, key string) int64 {
	v, ok := args[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return n
	default:
		return 0
	}
}

func splitKeywords(raw string) []string {
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func sourceTypeFromURL(u string) string {
	lower := strings.ToLower(u)
	switch {
	case strings.Contains(lower, "facebook.com") || strings.Contains(lower, "fb.com"):
		if strings.Contains(lower, "/posts/") || strings.Contains(lower, "story_fbid") || strings.Contains(lower, "/permalink/") {
			return "facebook_post"
		}
		return "facebook_group"
	default:
		return "web_url"
	}
}

func detectPlatformFromURL(u string) models.Platform {
	lower := strings.ToLower(u)
	switch {
	case strings.Contains(lower, "facebook.com") || strings.Contains(lower, "fb.com"):
		return models.PlatformFacebook
	case strings.Contains(lower, "tiktok.com"):
		return models.PlatformTikTok
	case strings.Contains(lower, "zalo"):
		return models.PlatformZalo
	default:
		return models.PlatformFacebook
	}
}
