package main

import (
	"log"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/config"
	"github.com/thg/scraper/internal/drivers/copilot"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/server"
	"github.com/thg/scraper/internal/skills"
	"github.com/thg/scraper/internal/store"
	tgclient "github.com/thg/scraper/internal/telegram/client"
)

// This file holds the external-service wiring helpers extracted from
// main() (2026-07-01) — AI agent, Telegram notifier, and HTTP serve. Same
// closure-free-extraction rationale as startup.go: pure setup logic moved
// out to keep the composition root under the cognitive-complexity guard,
// with no behavior change.

// setupAIAgent wires the AI Agent (OpenAI Function Calling v2) — two
// MessageGenerator instances on purpose: classifierMg is the high-volume,
// schema-locked classifier (cheap/fast model), commentMg is the user-facing
// comment/inbox/post generator (strong model). notify is an already-
// constructed closure (built in main() around its own telegramNotify
// variable) passed in by value — extraction does not change what it closes
// over. Returns (nil, nil, nil) when OPENAI_API_KEY is unset, exactly
// matching the inline version's disabled-agent behavior.
func setupAIAgent(cfg *config.Config, db *store.Store, jobStore *jobs.Store, notify func(string)) (*copilot.Agent, *ai.MessageGenerator, *ai.MessageGenerator) {
	if cfg.OpenAIAPIKey == "" {
		log.Println("⚠️  OPENAI_API_KEY not set, AI Agent disabled")
		return nil, nil, nil
	}
	classifierMg := ai.NewMessageGenerator(cfg.OpenAIAPIKey, cfg.OpenAIClassifierModel)
	commentMg := ai.NewMessageGenerator(cfg.OpenAIAPIKey, cfg.OpenAICommentModel)
	agent := copilot.NewAgent(cfg.OpenAIAPIKey, cfg.OpenAICommentModel, db)
	if cfg.AgentBrainURL != "" {
		agent.SetBrainClient(copilot.NewBrainClient(cfg.AgentBrainURL, time.Duration(cfg.AgentBrainTimeout)*time.Millisecond))
		log.Printf("✅ Agent Brain sidecar enabled: %s", cfg.AgentBrainURL)
	}
	actionHandler := makeAgentActionHandler(db, jobStore, commentMg, notify)
	agent.ActionHandler = actionHandler

	// Phase 6: register the open-prompt skill catalog. Each skill captures
	// the action handler so its Run closure can re-route into the existing
	// production logic without duplicating it.
	skillRegistry := skills.NewRegistry()
	registerBuiltinSkills(skillRegistry, builtinSkillDeps{
		db:       db,
		jobStore: jobStore,
		msgGen:   commentMg,
		notify:   notify,
		handler:  actionHandler,
	})
	agent.SetSkillRegistry(skillRegistry)
	log.Printf("✅ AI Agent initialized (classifier: %s, comment: %s, skills=%d)",
		cfg.OpenAIClassifierModel, cfg.OpenAICommentModel, len(skillRegistry.All()))
	return agent, classifierMg, commentMg
}

// setupTelegramNotifier wires the system notifier to the configured admin
// chat via the thin Telegram client. The tenant-scoped webhook control-plane
// (POST /api/telegram/webhook, wired in the server) is the SINGLE Telegram
// runtime; this is only the admin-alert notifier. Returns nil when Telegram
// is not configured, matching the inline version's disabled-notifier state.
func setupTelegramNotifier(cfg *config.Config) func(string) {
	if cfg.TelegramBotToken == "" {
		log.Println("⚠️  Telegram bot token not set, Telegram disabled")
		return nil
	}
	tgClient := tgclient.New(cfg.TelegramBotToken)
	var notifier func(string)
	if cfg.TelegramAdminChat != 0 {
		notifier = func(msg string) { _ = tgClient.Send(cfg.TelegramAdminChat, msg) }
	}
	log.Println("✅ Telegram client initialized (webhook runtime; legacy long-poll bot retired)")
	return notifier
}

// serveHTTP starts the web server and logs a warning if it exits with an
// error. Called via `go serveHTTP(srv)` at the exact site the inline
// goroutine literal used to run — only the closure's location moved.
func serveHTTP(srv *server.Server) {
	if err := srv.Start(); err != nil {
		log.Printf("⚠️  Web server error: %v", err)
	}
}
