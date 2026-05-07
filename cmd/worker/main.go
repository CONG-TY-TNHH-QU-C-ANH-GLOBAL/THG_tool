package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/thg/scraper/internal/ai"
	facebookcrawl "github.com/thg/scraper/internal/jobhandlers/facebook_crawl"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/livesession"
	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("⚙️  THG Worker — Starting...")

	_ = godotenv.Load()

	dbPath := envOr("DB_PATH", "data/scraper.db")

	// ── Job store (scheduler_jobs table, separate WAL connection) ────────────
	jobStore, err := jobs.NewStore(dbPath)
	if err != nil {
		log.Fatalf("❌ job store: %v", err)
	}
	log.Println("✅ Job store opened (scheduler_jobs)")

	// ── Main store + AppStore (app_tasks, task_leads) ────────────────────────
	mainStore, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("❌ main store: %v", err)
	}
	defer mainStore.Close()

	appStore, err := store.NewAppStore(mainStore)
	if err != nil {
		log.Fatalf("❌ app store: %v", err)
	}
	log.Println("✅ App store opened (app_tasks, task_leads)")

	// ── Session allocator + live session factory ─────────────────────────────
	// The allocator atomically claims idle browser sessions per job.
	// When no session is idle, jobs fail loudly unless ALLOW_MOCK_RUNTIME=true.
	rawDB := appStore.DB()
	sm := session.NewStateMachine(rawDB)
	allocator := session.NewAllocator(rawDB, sm)
	lsFactory := livesession.NewLiveSessionFactory(rawDB, appStore, allocator)
	log.Println("✅ Session allocator initialized")

	// ── Handler ──────────────────────────────────────────────────────────────
	// MockRuntime is opt-in only. In production, missing browser sessions must
	// fail loudly instead of producing fake leads.
	var fallback runtime.Runtime
	if envOr("ALLOW_MOCK_RUNTIME", "false") == "true" {
		fallback = runtime.NewMockRuntime()
		log.Println("⚠️  ALLOW_MOCK_RUNTIME=true — worker may emit mock crawl data")
	}
	scorer := scoring.New(scoring.DefaultConfig())
	h := facebookcrawl.New(fallback, scorer, jobStore, appStore)
	h.SetAllocator(allocator, lsFactory)
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		// OPENAI_CLASSIFIER_MODEL is the canonical name; OPENAI_MODEL is the
		// legacy alias kept for backwards compat with /etc/thg-scraper/env on
		// production VPS. cmd/scraper/main.go reads the same pair via
		// config.OpenAIClassifierModel — keep the resolution order in sync.
		model := envOr("OPENAI_CLASSIFIER_MODEL", envOr("OPENAI_MODEL", "gpt-4o-mini"))
		h.SetUniversalClassifier(mainStore, ai.NewMessageGenerator(apiKey, model))
		log.Printf("✅ Universal AI classifier enabled (model: %s)", model)
	}

	// ── Registry — map every open crawler intent the API/Telegram can submit ──
	registry := jobs.NewRegistry()

	// Production uses prompt-open intents. Legacy "scrape_group" can be enabled
	// only to drain old queued jobs during migration.
	intents := []string{
		"facebook_crawl", // explicit browser crawl intent
		"facebook_group", // alias used by some skill routes
		"lead_gen",       // generic lead-generation intent
		"web_crawl",      // generic web crawl
	}
	if envOr("ENABLE_LEGACY_SCRAPE_GROUP", "false") == "true" {
		intents = append(intents, "scrape_group")
	}
	for _, intent := range intents {
		registry.Register(intent, h)
	}
	log.Printf("✅ %d intents registered → facebook_crawl handler", len(intents))

	// ── Scheduler ────────────────────────────────────────────────────────────
	// Polls scheduler_jobs every 500 ms, claims pending rows atomically,
	// dispatches to the registered handler in a goroutine, and writes
	// running → completed / failed back to the DB.
	sched := jobs.NewScheduler(jobStore, registry)

	ctx, cancel := context.WithCancel(context.Background())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("🛑 Worker: signal received, shutting down...")
		cancel()
	}()

	log.Println("🚀 Worker ready — polling scheduler_jobs every 500 ms")
	sched.Run(ctx) // blocks until ctx cancelled
	log.Println("✅ Worker stopped cleanly")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
