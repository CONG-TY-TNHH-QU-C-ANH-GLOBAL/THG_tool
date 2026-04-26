package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	facebookcrawl "github.com/thg/scraper/internal/handlers/facebook_crawl"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/store"
)

func main() {
	dbPath := env("DB_PATH", "data/scraper.db")

	jobStore, err := jobs.NewStore(dbPath)
	if err != nil {
		log.Fatalf("open job store: %v", err)
	}

	legacyStore, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("open legacy store: %v", err)
	}
	appStore, err := store.NewAppStore(legacyStore)
	if err != nil {
		log.Fatalf("init app store: %v", err)
	}

	scorer := scoring.New(scoring.DefaultConfig())
	rt := runtime.NewMockRuntime()

	newHandler := func() *facebookcrawl.Handler {
		return facebookcrawl.New(rt, scorer, jobStore, appStore)
	}

	registry := jobs.NewRegistry()
	registry.Register("facebook_crawl", newHandler())
	registry.Register("lead_gen", newHandler())
	registry.Register("visa_research", newHandler())
	registry.Register("web_crawl", newHandler())

	scheduler := jobs.NewScheduler(jobStore, registry)

	ctx, cancel := context.WithCancel(context.Background())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("worker: shutting down")
		cancel()
	}()

	log.Println("worker: started (mock runtime, 4 intents registered)")
	scheduler.Run(ctx)
	log.Println("worker: stopped")
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
