package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thg/scraper/internal/api"
	"github.com/thg/scraper/internal/events"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/parser"
	"github.com/thg/scraper/internal/store"
)

func main() {
	dbPath := env("DB_PATH", "data/scraper.db")
	addr := ":" + env("API_PORT", "8080")

	jobStore, err := jobs.NewStore(dbPath)
	if err != nil {
		log.Fatalf("open job store: %v", err)
	}

	// AppStore wraps the same SQLite DB via the legacy store.
	legacyStore, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("open legacy store: %v", err)
	}
	appStore, err := store.NewAppStore(legacyStore)
	if err != nil {
		log.Fatalf("init app store: %v", err)
	}

	bus := events.NewBus()
	p := parser.NewRuleBasedParser()
	srv := api.New(jobStore, appStore, p, bus)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.Start(ctx)

	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      srv,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second, // SSE connections are long-lived
		IdleTimeout:  120 * time.Second,
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}

	go func() {
		log.Printf("api: listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("api: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := httpSrv.Shutdown(shutCtx); err != nil {
		log.Printf("api: shutdown error: %v", err)
	}
	log.Println("api: stopped")
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
