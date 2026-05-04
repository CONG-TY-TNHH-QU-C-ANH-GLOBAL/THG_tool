package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"regexp"
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
	"github.com/thg/scraper/internal/mailer"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server"
	session_pkg "github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/skills"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram"
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

	// Reset any orphaned approved outbound messages from previous crashes/restarts
	if err := db.ResetOrphanedOutbounds(); err != nil {
		log.Printf("⚠️ Failed to reset orphaned outbounds: %v", err)
	}

	// Recover local-runtime jobs that an agent claimed but never finished
	// before the API restarted. Without this, those rows stay 'running'
	// forever and never become eligible for re-claim. 10 min is long
	// enough that genuine slow scrapes aren't preempted.
	if recovered, err := db.RecoverStaleLocalJobs(10 * time.Minute); err != nil {
		log.Printf("⚠️ Failed to recover stale local jobs: %v", err)
	} else if recovered > 0 {
		log.Printf("✅ Recovered %d stale local jobs from prior runtime claims", recovered)
	}
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := db.RecoverStaleLocalJobs(10 * time.Minute); err != nil {
					log.Printf("⚠️ stale local-job recovery: %v", err)
				}
			}
		}
	}()

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

	// Initialize price extractor (OpenAI Vision for reading price list images)
	var pricer *ai.PriceExtractor
	if cfg.OpenAIAPIKey != "" {
		pricer = ai.NewPriceExtractor(cfg.OpenAIAPIKey, cfg.OpenAIModel)
		log.Println("✅ Price extractor initialized")
	}

	// Initialize AI Agent (OpenAI Function Calling) — v2
	var telegramNotify func(string)
	var agent *ai.Agent
	var msgGen *ai.MessageGenerator
	skillRegistry := skills.NewRegistry()
	if cfg.OpenAIAPIKey != "" {
		msgGen = ai.NewMessageGenerator(cfg.OpenAIAPIKey, cfg.OpenAICommentModel)
		agent = ai.NewAgent(cfg.OpenAIAPIKey, cfg.OpenAICommentModel, db)
		if cfg.AgentBrainURL != "" {
			agent.SetBrainClient(ai.NewBrainClient(cfg.AgentBrainURL, time.Duration(cfg.AgentBrainTimeout)*time.Millisecond))
			log.Printf("✅ Agent Brain sidecar enabled: %s", cfg.AgentBrainURL)
		}
		actionHandler := makeAgentActionHandler(db, jobStore, msgGen, func(msg string) {
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
			msgGen:   msgGen,
			notify: func(msg string) {
				if telegramNotify != nil {
					telegramNotify(msg)
				}
			},
			handler: actionHandler,
		})
		agent.SetSkillRegistry(skillRegistry)
		log.Printf("✅ AI Agent initialized (model: %s, skills=%d)", cfg.OpenAICommentModel, len(skillRegistry.All()))
	} else {
		log.Println("⚠️  OPENAI_API_KEY not set, AI Agent disabled")
	}

	// Initialize Telegram bot (optional)
	var bot *telegram.Bot
	if cfg.TelegramBotToken != "" {
		bot, err = telegram.New(cfg.TelegramBotToken, cfg.TelegramAdminChat, db, jobStore, agent, pricer)
		if bot != nil {
			bot.SetDefaultOrgID(cfg.TelegramOrgID)
		}
		if err != nil {
			log.Printf("⚠️  Telegram bot init failed: %v", err)
		} else {
			telegramNotify = bot.Notify
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

	go runCrawlIntentScheduler(ctx, db, jobStore, time.Minute)
	log.Println("✅ Recurring crawl intent scheduler started (org plans → 30m+ automation)")

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

func makeAgentActionHandler(db *store.Store, jobStore *jobs.Store, msgGen *ai.MessageGenerator, notify func(string)) func(string, map[string]any) (string, error) {
	return func(action string, args map[string]any) (string, error) {
		switch action {
		case "set_context":
			key, value := argString(args, "key"), argString(args, "value")
			if key == "" || value == "" {
				return "", fmt.Errorf("set_context requires key and value")
			}
			// Approval policy keys (outbound_mode, auto_comment_mode) are
			// admin-controlled. AI tools must NOT be able to flip the org
			// into auto-execute via prompt — that is exactly the
			// prompt-injection vector flagged in the 2026-05-03 audit.
			// Operators set outbound_mode via the dashboard / admin API.
			switch key {
			case "outbound_mode", "auto_comment_mode", "org:outbound_mode":
				return "", fmt.Errorf("outbound_mode is admin-controlled; ask the workspace owner to change it in Dashboard › Settings, not via AI prompt")
			}
			if orgID := argInt64(args, "org_id"); orgID > 0 {
				switch key {
				case "business_profile", "private_files_summary", "data_sources_summary":
					key = fmt.Sprintf("org:%d:%s", orgID, key)
				}
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
			key := "business_desc"
			if orgID := argInt64(args, "org_id"); orgID > 0 {
				key = fmt.Sprintf("org:%d:business_profile", orgID)
			}
			if err := db.SetContext(key, desc); err != nil {
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
				OrgID:     argInt64(args, "org_id"),
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
			return submitOpenCrawl(context.Background(), db, jobStore, "facebook_crawl", []jobs.Source{{Type: sourceTypeFromURL(u), URL: u, Label: "prompt_url"}}, args)
		case "scrape_comments":
			u := argString(args, "post_url")
			if u == "" {
				return "", fmt.Errorf("scrape_comments requires post_url")
			}
			return submitOpenCrawl(context.Background(), db, jobStore, "facebook_crawl", []jobs.Source{{Type: "facebook_post", URL: u, Label: "prompt_post"}}, args)
		case "classify_leads":
			return "classification runs inline during every crawler job using the current business context", nil
		case "search_groups":
			query := argString(args, "query")
			if query == "" {
				return "", fmt.Errorf("search_groups requires query")
			}
			searchURL := "https://www.facebook.com/search/groups/?q=" + url.QueryEscape(query)
			return submitOpenCrawl(context.Background(), db, jobStore, "facebook_crawl", []jobs.Source{{Type: "facebook_search", URL: searchURL, Label: "group_search"}}, args)
		case "auto_comment", "comment_all_leads":
			return queueLeadOutreach(context.Background(), db, msgGen, "comment", args, notify)
		case "auto_inbox", "inbox_all_leads":
			return queueLeadOutreach(context.Background(), db, msgGen, "inbox", args, notify)
		case "create_job_post":
			return queueGroupPost(context.Background(), db, msgGen, args, notify)
		default:
			return "", fmt.Errorf("agent action %q is not wired to a production handler yet", action)
		}
	}
}

func submitOpenCrawl(ctx context.Context, db *store.Store, jobStore *jobs.Store, intent string, sources []jobs.Source, args map[string]any) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("crawler requires at least one source")
	}
	maxItems := int(argInt64(args, "max_items"))
	if maxItems <= 0 {
		maxItems = int(argInt64(args, "limit"))
	}
	if maxItems <= 0 {
		maxItems = maxItemsFromPrompt(argString(args, "user_prompt"))
	}
	if maxItems <= 0 {
		maxItems = 50
	}
	keywords := splitKeywords(argString(args, "keywords"))
	if len(keywords) == 0 {
		keywords = splitKeywords(promptKeywordFallback(argString(args, "user_prompt")))
	}
	orgID := argInt64(args, "org_id")
	accountID := argInt64(args, "account_id")
	if accountID <= 0 && orgID > 0 && db != nil {
		if pickedAccountID, err := pickReadyFacebookAccountIDForCrawl(db, orgID); err == nil && pickedAccountID > 0 {
			accountID = pickedAccountID
			args["account_id"] = pickedAccountID
		}
	}
	task := &jobs.Task{
		SchemaVersion: "1",
		TaskID:        openCrawlTaskID(intent, sources, args),
		OrgID:         orgID,
		AccountID:     accountID,
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
	if db != nil && !argBool(args, "_recurring_run") {
		rememberRecurringCrawlIntents(ctx, db, task, args)
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return "", err
	}
	if db != nil {
		if result, routed, err := submitLocalRuntimeCrawl(ctx, db, task, string(payload)); routed {
			return result, err
		}
	}
	job, err := jobStore.Submit(ctx, task, string(payload))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("da tao crawler job #%d task=%s intent=%s", job.ID, job.TaskID, intent), nil
}

func pickReadyFacebookAccountIDForCrawl(db *store.Store, orgID int64) (int64, error) {
	screen, err := db.GetLatestConnectorScreenshot(orgID, 0)
	if err != nil {
		return 0, err
	}
	if screen != nil &&
		screen.AccountID > 0 &&
		screen.AgentID > 0 &&
		strings.EqualFold(strings.TrimSpace(screen.StreamStatus), "facebook_logged_in") &&
		time.Since(screen.UpdatedAt) <= 5*time.Minute {
		return screen.AccountID, nil
	}
	accounts, err := db.GetAllAccounts(orgID)
	if err != nil {
		return 0, err
	}
	for _, acc := range accounts {
		if acc.Platform == models.PlatformFacebook &&
			acc.BrowserLoggedIn &&
			acc.Status == models.AccountActive &&
			strings.TrimSpace(acc.FBUserID) != "" {
			return acc.ID, nil
		}
	}
	return 0, nil
}

func submitLocalRuntimeCrawl(ctx context.Context, db *store.Store, task *jobs.Task, payload string) (string, bool, error) {
	if task == nil || task.OrgID <= 0 || task.AccountID <= 0 {
		return "", false, nil
	}
	screen, err := db.GetLatestConnectorScreenshot(task.OrgID, task.AccountID)
	if err != nil {
		return "", true, err
	}
	if screen != nil && screen.AgentID > 0 && strings.EqualFold(strings.TrimSpace(screen.StreamStatus), "facebook_logged_in") && time.Since(screen.UpdatedAt) <= 5*time.Minute {
		result, err := enqueueLocalRuntimeCrawlCommand(ctx, db, task, payload, screen.AgentID)
		return result, true, err
	}
	if agentID, reason := pickOnlineRuntimeAgentForCrawl(db, task); agentID > 0 {
		result, err := enqueueLocalRuntimeCrawlCommand(ctx, db, task, payload, agentID)
		return result, true, err
	} else if reason != "" {
		log.Printf("[LocalCrawl] no heartbeat-routable runtime org=%d account=%d: %s", task.OrgID, task.AccountID, reason)
	}

	appStore, err := store.NewAppStore(db)
	if err != nil {
		return "", true, err
	}
	sess, _ := appStore.GetSession(ctx, task.AccountID)
	if sess != nil && sess.CDPPort > 0 && (sess.Status == "idle" || sess.Status == "ready" || sess.Status == "active") {
		return "", false, nil
	}
	return "", true, fmt.Errorf("Facebook account #%d is saved, but THG Local Runtime is not online for this account yet. Open Browser, run THG Local Kit, wait for Facebook local ready, then send the prompt again", task.AccountID)
}

func enqueueLocalRuntimeCrawlCommand(ctx context.Context, db *store.Store, task *jobs.Task, payload string, agentID int64) (string, error) {
	if agentID <= 0 {
		return "", fmt.Errorf("local runtime agent id is required")
	}
	appStore, err := store.NewAppStore(db)
	if err != nil {
		return "", err
	}
	_ = appStore.CreateTask(ctx, task.TaskID, task.OrgID, task.Intent)
	_ = appStore.StartTask(ctx, task.TaskID)
	cmdID, err := db.CreateConnectorCommand(task.OrgID, task.AccountID, agentID, 0, "crawl", payload)
	if err != nil {
		_ = appStore.FailTask(ctx, task.TaskID, err.Error())
		return "", err
	}
	return fmt.Sprintf("da tao local crawler command #%d task=%s intent=%s mode=local_runtime", cmdID, task.TaskID, task.Intent), nil
}

func pickOnlineRuntimeAgentForCrawl(db *store.Store, task *jobs.Task) (int64, string) {
	connectors, err := db.ListLocalConnectors(task.OrgID)
	if err != nil {
		return 0, err.Error()
	}
	if len(connectors) == 0 {
		return 0, "no local connector paired"
	}
	acc, _ := db.GetAccountForOrg(task.AccountID, task.OrgID)
	var reasons []string
	for _, conn := range connectors {
		if !conn.Online {
			reasons = append(reasons, fmt.Sprintf("connector #%d offline", conn.ID))
			continue
		}
		if conn.AssignedAccountID > 0 && conn.AssignedAccountID != task.AccountID {
			reasons = append(reasons, fmt.Sprintf("connector #%d assigned to account #%d", conn.ID, conn.AssignedAccountID))
			continue
		}
		status := strings.TrimSpace(conn.StreamStatus)
		if !strings.EqualFold(status, "facebook_logged_in") {
			reasons = append(reasons, fmt.Sprintf("connector #%d status=%s", conn.ID, firstNonEmpty(status, "unknown")))
			continue
		}
		if strings.TrimSpace(conn.FBUserID) == "" {
			reasons = append(reasons, fmt.Sprintf("connector #%d missing fb_user_id", conn.ID))
			continue
		}
		if acc != nil && strings.TrimSpace(acc.FBUserID) != "" && strings.TrimSpace(conn.FBUserID) != strings.TrimSpace(acc.FBUserID) {
			reasons = append(reasons, fmt.Sprintf("connector #%d fb_user_id mismatch", conn.ID))
			continue
		}
		return conn.ID, ""
	}
	return 0, strings.Join(reasons, "; ")
}

func rememberRecurringCrawlIntents(ctx context.Context, db *store.Store, task *jobs.Task, args map[string]any) {
	if db == nil || task == nil || task.OrgID <= 0 || task.AccountID <= 0 {
		return
	}
	prompt := argString(args, "user_prompt")
	intervalMinutes := int(argInt64(args, "interval_minutes"))
	maxItems := task.CrawlPlan.MaxItems
	for _, src := range task.CrawlPlan.Sources {
		if !isRecurringCrawlSource(src) {
			continue
		}
		intent, err := db.UpsertCrawlIntent(ctx, store.CrawlIntent{
			OrgID:           task.OrgID,
			AccountID:       task.AccountID,
			Name:            firstNonEmpty(argString(args, "name"), argString(args, "query")),
			Prompt:          prompt,
			Intent:          task.Intent,
			SourceType:      src.Type,
			SourceURL:       src.URL,
			SourceLabel:     src.Label,
			Keywords:        task.Keywords,
			IntervalMinutes: intervalMinutes,
			MaxItems:        maxItems,
		})
		if err != nil {
			log.Printf("[CrawlIntent] remember failed org=%d account=%d source=%s: %v", task.OrgID, task.AccountID, src.URL, err)
			continue
		}
		log.Printf("[CrawlIntent] remembered org=%d account=%d intent=%d interval=%dm source=%s", intent.OrgID, intent.AccountID, intent.ID, intent.IntervalMinutes, intent.SourceURL)
	}
}

func isRecurringCrawlSource(src jobs.Source) bool {
	switch strings.ToLower(strings.TrimSpace(src.Type)) {
	case "facebook_group", "facebook_search", "web_url":
		return strings.TrimSpace(src.URL) != ""
	default:
		return false
	}
}

func runCrawlIntentScheduler(ctx context.Context, db *store.Store, jobStore *jobs.Store, tickEvery time.Duration) {
	if db == nil || jobStore == nil {
		return
	}
	if tickEvery <= 0 {
		tickEvery = time.Minute
	}
	run := func() {
		if err := scheduleDueCrawlIntents(ctx, db, jobStore); err != nil {
			log.Printf("[CrawlIntent] scheduler error: %v", err)
		}
	}
	run()
	ticker := time.NewTicker(tickEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func scheduleDueCrawlIntents(ctx context.Context, db *store.Store, jobStore *jobs.Store) error {
	now := time.Now().UTC()
	intents, err := db.ClaimDueCrawlIntents(ctx, now, 10)
	if err != nil {
		return err
	}
	for _, intent := range intents {
		accountID := intent.AccountID
		if accountID <= 0 {
			if picked, pickErr := pickReadyFacebookAccountIDForCrawl(db, intent.OrgID); pickErr == nil {
				accountID = picked
			}
		}
		taskID := recurringCrawlTaskID(intent.ID, now, intent.IntervalMinutes)
		if accountID <= 0 {
			errMsg := "no ready Facebook account for recurring crawl"
			_ = db.MarkCrawlIntentRunResult(ctx, intent.ID, taskID, errMsg)
			log.Printf("[CrawlIntent] skipped intent=%d org=%d: %s", intent.ID, intent.OrgID, errMsg)
			continue
		}
		args := map[string]any{
			"org_id":         intent.OrgID,
			"account_id":     accountID,
			"keywords":       strings.Join(intent.Keywords, ", "),
			"max_items":      intent.MaxItems,
			"user_prompt":    intent.Prompt,
			"_recurring_run": true,
			"_task_id":       taskID,
		}
		source := jobs.Source{Type: intent.SourceType, URL: intent.SourceURL, Label: firstNonEmpty(intent.SourceLabel, "recurring_intent")}
		result, submitErr := submitOpenCrawl(ctx, db, jobStore, intent.Intent, []jobs.Source{source}, args)
		errMsg := ""
		if submitErr != nil {
			errMsg = submitErr.Error()
		}
		if err := db.MarkCrawlIntentRunResult(ctx, intent.ID, taskID, errMsg); err != nil {
			log.Printf("[CrawlIntent] mark result failed intent=%d: %v", intent.ID, err)
		}
		if submitErr != nil {
			log.Printf("[CrawlIntent] run failed intent=%d task=%s: %v", intent.ID, taskID, submitErr)
			continue
		}
		log.Printf("[CrawlIntent] scheduled intent=%d task=%s: %s", intent.ID, taskID, result)
	}
	return nil
}

func recurringCrawlTaskID(intentID int64, now time.Time, intervalMinutes int) string {
	if intervalMinutes <= 0 {
		intervalMinutes = 30
	}
	bucketSeconds := int64(intervalMinutes * 60)
	if bucketSeconds <= 0 {
		bucketSeconds = 1800
	}
	return fmt.Sprintf("autocrawl-%d-%d", intentID, now.UTC().Unix()/bucketSeconds)
}

func queueLeadOutreach(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, msgType string, args map[string]any, notify func(string)) (string, error) {
	orgID := argInt64(args, "org_id")
	if orgID <= 0 {
		return "", fmt.Errorf("org_id is required for outbound automation")
	}
	accountID := argInt64(args, "account_id")
	if accountID <= 0 {
		accounts, err := db.GetAllAccounts(orgID)
		if err != nil {
			return "", err
		}
		for _, acc := range accounts {
			if acc.Platform == models.PlatformFacebook && acc.BrowserLoggedIn && acc.Status == models.AccountActive {
				accountID = acc.ID
				break
			}
		}
		if accountID <= 0 && len(accounts) > 0 {
			accountID = accounts[0].ID
		}
	}
	if accountID <= 0 {
		return "", fmt.Errorf("no Facebook account available for org %d", orgID)
	}

	// requestedAuto carries the AI/agent's preference. The store layer
	// (QueueOutboundForOrg → IsAutoOutboundEnabledForOrg) is the final
	// gatekeeper — it will downgrade to draft if the org hasn't opted in.
	requestedAuto := argBool(args, "auto")

	leads, err := leadsFromActionArgs(db, orgID, msgType, args)
	if err != nil {
		return "", err
	}
	if len(leads) == 0 {
		return "khong co lead phu hop de queue outbound", nil
	}

	businessContext := businessContextForOrg(db, orgID)
	template := argString(args, "template")
	queued, skipped := 0, 0
	approvedCount := 0
	skipReasons := map[string]int{}
	for _, lead := range leads {
		targetURL := strings.TrimSpace(lead.SourceURL)
		profileURL := strings.TrimSpace(lead.AuthorURL)
		if msgType == "inbox" {
			targetURL = profileURL
		}
		if targetURL == "" {
			skipped++
			skipReasons["missing_target"]++
			continue
		}

		content := template
		if msgGen != nil && msgGen.Available() {
			genCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
			var genErr error
			if template != "" && msgType == "comment" {
				content, genErr = msgGen.GenerateCommentFromTemplate(genCtx, template, lead.Content, lead.Author)
			} else if msgType == "comment" {
				content, genErr = msgGen.GenerateCommentWithService(genCtx, lead.Content, lead.Author, businessContext, lead.ServiceMatch, "")
			} else {
				content, genErr = msgGen.GenerateInboxMessage(genCtx, lead.Content, lead.Author, businessContext, "")
			}
			cancel()
			if genErr != nil {
				skipped++
				skipReasons["generation_failed"]++
				continue
			}
		}
		content = strings.TrimSpace(content)
		if content == "" {
			skipped++
			skipReasons["empty_content"]++
			continue
		}

		result, err := db.QueueOutboundForOrg(&models.OutboundMessage{
			OrgID:      orgID,
			Type:       msgType,
			Platform:   models.PlatformFacebook,
			AccountID:  accountID,
			TargetURL:  targetURL,
			TargetName: lead.Author,
			Content:    content,
			Context:    lead.Content,
			AIModel:    "agent",
		}, requestedAuto, 24*time.Hour)
		if err != nil {
			return "", err
		}
		if !result.Decision.Allowed {
			skipped++
			skipReasons[result.Decision.Reason]++
			continue
		}
		queued++
		if result.Status == models.OutboundApproved {
			approvedCount++
		}
	}

	mode := "draft"
	switch {
	case approvedCount > 0 && approvedCount == queued:
		mode = "approved_auto"
	case approvedCount > 0:
		mode = "mixed"
	case requestedAuto:
		// Caller asked for auto but the org isn't opted in — make this
		// visible in the response so the operator knows why it queued as draft.
		mode = "draft_org_not_auto"
	}
	if notify != nil && queued > 0 {
		notify(formatOutboundNotification(orgID, accountID, msgType, queued, skipped, mode))
	}
	return fmt.Sprintf("queued_%s=%d skipped=%d mode=%s reasons=%v", msgType, queued, skipped, mode, skipReasons), nil
}

func leadsFromActionArgs(db *store.Store, orgID int64, msgType string, args map[string]any) ([]models.Lead, error) {
	if msgType == "comment" {
		if target := firstNonEmpty(argString(args, "post_url"), argString(args, "target_url")); target != "" {
			return []models.Lead{{
				OrgID:      orgID,
				SourceURL:  target,
				Author:     argString(args, "target_name"),
				AuthorURL:  argString(args, "author_url"),
				Content:    argString(args, "context"),
				Score:      models.LeadHot,
				Platform:   models.PlatformFacebook,
				SourceType: "prompt_target",
			}}, nil
		}
	} else if target := argString(args, "target_url"); target != "" {
		return []models.Lead{{
			OrgID:      orgID,
			AuthorURL:  target,
			Author:     argString(args, "target_name"),
			Content:    argString(args, "context"),
			Score:      models.LeadHot,
			Platform:   models.PlatformFacebook,
			SourceType: "prompt_target",
		}}, nil
	}
	score := argString(args, "score_filter")
	if score == "" && msgType == "inbox" {
		score = "hot"
	}
	limit := int(argInt64(args, "limit"))
	if limit <= 0 {
		limit = 25
	}
	return db.GetAutomationLeadsForOrg(orgID, score, limit)
}

func queueGroupPost(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, args map[string]any, notify func(string)) (string, error) {
	orgID := argInt64(args, "org_id")
	if orgID <= 0 {
		return "", fmt.Errorf("org_id is required for group posting")
	}
	accountID := argInt64(args, "account_id")
	if accountID <= 0 {
		accounts, err := db.GetAllAccounts(orgID)
		if err != nil {
			return "", err
		}
		if len(accounts) == 0 {
			return "", fmt.Errorf("no Facebook account available for org %d", orgID)
		}
		accountID = accounts[0].ID
	}

	content := firstNonEmpty(argString(args, "content"), argString(args, "description"), argString(args, "title"))
	if msgGen != nil && msgGen.Available() && argString(args, "title") != "" {
		genCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		generated, err := msgGen.GenerateJobPost(genCtx,
			argString(args, "title"),
			argString(args, "description"),
			argString(args, "requirements"),
			argString(args, "benefits"),
			argString(args, "salary"),
			argString(args, "email"),
		)
		cancel()
		if err == nil && strings.TrimSpace(generated) != "" {
			content = generated
		}
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("group post content is required")
	}

	targets := []string{}
	if u := argString(args, "group_url"); u != "" {
		targets = append(targets, u)
	} else {
		groups, err := db.GetAllGroups(orgID)
		if err != nil {
			return "", err
		}
		for _, g := range groups {
			if g.Active && strings.TrimSpace(g.URL) != "" {
				targets = append(targets, g.URL)
				if len(targets) >= 3 {
					break
				}
			}
		}
	}
	if len(targets) == 0 {
		return "khong co group target de queue group_post", nil
	}

	requestedAuto := argBool(args, "auto")
	queued, skipped := 0, 0
	approvedCount := 0
	for _, target := range targets {
		result, err := db.QueueOutboundForOrg(&models.OutboundMessage{
			OrgID:     orgID,
			Type:      "group_post",
			Platform:  models.PlatformFacebook,
			AccountID: accountID,
			TargetURL: target,
			Content:   strings.TrimSpace(content),
			AIModel:   "agent",
		}, requestedAuto, 24*time.Hour)
		if err != nil {
			return "", err
		}
		if !result.Decision.Allowed {
			skipped++
			continue
		}
		queued++
		if result.Status == models.OutboundApproved {
			approvedCount++
		}
	}
	mode := "draft"
	switch {
	case approvedCount > 0 && approvedCount == queued:
		mode = "approved_auto"
	case approvedCount > 0:
		mode = "mixed"
	case requestedAuto:
		mode = "draft_org_not_auto"
	}
	if notify != nil && queued > 0 {
		notify(formatOutboundNotification(orgID, accountID, "group_post", queued, skipped, mode))
	}
	return fmt.Sprintf("queued_group_posts=%d skipped=%d mode=%s", queued, skipped, mode), nil
}

func formatOutboundNotification(orgID, accountID int64, msgType string, queued, skipped int, mode string) string {
	label := "outbound"
	switch msgType {
	case "comment":
		label = "Facebook comments"
	case "inbox":
		label = "Facebook inbox"
	case "group_post":
		label = "Facebook posting"
	}
	state := "drafts waiting for approval"
	if mode == "approved_auto" {
		state = "approved for local runtime execution"
	}
	return fmt.Sprintf("[THG Agent] %s queued: %d (%s). Org #%d, account #%d, skipped %d by guardrails.", label, queued, state, orgID, accountID, skipped)
}

func openCrawlTaskID(intent string, sources []jobs.Source, args map[string]any) string {
	if taskID := argString(args, "_task_id"); strings.HasPrefix(taskID, "autocrawl-") {
		return taskID
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s|day=%s|", intent, time.Now().UTC().Format("2006-01-02"))
	for _, src := range sources {
		fmt.Fprintf(h, "%s:%s|", src.Type, src.URL)
	}
	fmt.Fprintf(h, "org=%d|account=%d", argInt64(args, "org_id"), argInt64(args, "account_id"))
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

func argBool(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "1" || s == "yes" || s == "auto"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
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

func maxItemsFromPrompt(prompt string) int {
	prompt = strings.ToLower(strings.TrimSpace(prompt))
	if prompt == "" {
		return 0
	}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(\d{1,3})\s*(?:bai|bài|post|posts)`),
		regexp.MustCompile(`(?:lay|lấy|cao|cào|crawl)\s*(\d{1,3})`),
	} {
		m := re.FindStringSubmatch(prompt)
		if len(m) != 2 {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n <= 0 {
			continue
		}
		if n > 200 {
			n = 200
		}
		return n
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func orgContext(db *store.Store, orgID int64, key string) string {
	value, _ := db.GetContext(fmt.Sprintf("org:%d:%s", orgID, key))
	return strings.TrimSpace(value)
}

func businessContextForOrg(db *store.Store, orgID int64) string {
	parts := []string{}
	for _, item := range []struct {
		label string
		key   string
	}{
		{"Business profile", "business_profile"},
		{"Sales voice", "sales_voice"},
		{"Sales voice memory", "sales_voice_summary"},
		{"Comment style rules", "comment_style_rules"},
		{"Inbox style rules", "inbox_style_rules"},
		{"CTA rules", "cta_rules"},
		{"Forbidden phrases", "forbidden_phrases"},
		{"Pricing summary", "pricing_summary"},
		{"Sales examples", "sales_examples_summary"},
		{"Target customers", "target_customers"},
		{"Target signals", "target_signals"},
		{"Reject rules", "reject_rules"},
		{"Private files", "private_files_summary"},
		{"Connected data sources", "data_sources_summary"},
	} {
		if value := orgContext(db, orgID, item.key); value != "" {
			parts = append(parts, item.label+":\n"+value)
		}
	}
	if price := strings.TrimSpace(db.GetPriceListText()); price != "" {
		parts = append(parts, price)
	}
	if len(parts) == 0 {
		return "Business context is not configured yet. Avoid making claims about prices, inventory, guarantees, or policies."
	}
	return strings.Join(parts, "\n\n")
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

func promptKeywordFallback(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = regexp.MustCompile(`https?://\S+`).ReplaceAllString(raw, " ")
	replacer := strings.NewReplacer("\n", " ", "\t", " ", ".", " ", ";", ",", ":", " ", "(", " ", ")", " ")
	raw = replacer.Replace(strings.ToLower(raw))
	stop := map[string]bool{
		"cào": true, "cao": true, "crawl": true, "scrape": true, "tôi": true, "toi": true,
		"cần": true, "can": true, "tìm": true, "tim": true, "tệp": true, "tep": true,
		"khách": true, "khach": true, "nhu": true, "cầu": true, "cau": true, "và": true, "va": true,
		"hoặc": true, "hoac": true, "the": true, "from": true, "with": true,
	}
	seen := map[string]bool{}
	out := make([]string, 0, 8)
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '|' || r == '/' }) {
		for _, part := range strings.Fields(token) {
			part = strings.Trim(part, " -_")
			if len([]rune(part)) < 3 || stop[part] || seen[part] {
				continue
			}
			seen[part] = true
			out = append(out, part)
			if len(out) >= 8 {
				return strings.Join(out, ", ")
			}
		}
	}
	return strings.Join(out, ", ")
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
