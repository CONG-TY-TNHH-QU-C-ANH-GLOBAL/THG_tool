package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	facebookcrawl "github.com/thg/scraper/internal/handlers/facebook_crawl"
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
	// When no session is idle, the handler falls back to MockRuntime.
	rawDB := appStore.DB()
	sm := session.NewStateMachine(rawDB)
	allocator := session.NewAllocator(rawDB, sm)
	lsFactory := livesession.NewLiveSessionFactory(rawDB, appStore, allocator)
	log.Println("✅ Session allocator initialized")

	// ── Handler ──────────────────────────────────────────────────────────────
	// MockRuntime is the fallback when no browser session is idle.
	scorer := scoring.New(scoring.DefaultConfig())
	h := facebookcrawl.New(runtime.NewMockRuntime(), scorer, jobStore, appStore)
	h.SetAllocator(allocator, lsFactory)

	// ── Registry — map every intent the API/Telegram can submit ─────────────
	registry := jobs.NewRegistry()

	// "scrape_group" is the default intent from Telegram /scan and POST /api/jobs.
	// All other intents map to the same facebook_crawl handler for now; swap
	// individual entries for specialised handlers as the skills layer matures.
	intents := []string{
		"scrape_group",   // Telegram /scan + API POST /api/jobs
		"facebook_crawl", // explicit browser crawl intent
		"facebook_group", // alias used by some skill routes
		"lead_gen",       // generic lead-generation intent
		"web_crawl",      // generic web crawl
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
