package crawl

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
)

// Routes registers crawl intent endpoints.
func Routes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	group.Get("/crawl-intents", listIntents(deps))
	// Legacy binary toggle — kept for back-compat with existing clients.
	// New clients should use the explicit state-transition routes below.
	group.Put("/crawl-intents/:id/enabled", adminOnly, setIntentEnabled(deps))
	// Explicit state-transition endpoints. status is the source of truth.
	// See project_scheduled_intelligence.md gap #4.
	group.Post("/crawl-intents/:id/pause", adminOnly, transitionIntent(deps, store.CrawlIntentStatusPaused))
	group.Post("/crawl-intents/:id/resume", adminOnly, transitionIntent(deps, store.CrawlIntentStatusActive))
	group.Post("/crawl-intents/:id/archive", adminOnly, transitionIntent(deps, store.CrawlIntentStatusArchived))
}
