package crawl

import (
	"github.com/gofiber/fiber/v2"

	crawlstore "github.com/thg/scraper/internal/store/crawl"
)

// Routes registers crawl intent endpoints.
func Routes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	group.Get("/crawl-intents", listIntents(deps))
	// User-facing creation surface (Missions UI in the FE). Idempotent via
	// store.UpsertCrawlIntent — re-POSTing the same URL refines the same row.
	group.Post("/crawl-intents", adminOnly, createIntent(deps))
	// Legacy binary toggle — kept for back-compat with existing clients.
	// New clients should use the explicit state-transition routes below.
	group.Put("/crawl-intents/:id/enabled", adminOnly, setIntentEnabled(deps))
	// Explicit state-transition endpoints. status is the source of truth.
	// See project_scheduled_intelligence.md gap #4.
	group.Post("/crawl-intents/:id/pause", adminOnly, transitionIntent(deps, crawlstore.IntentStatusPaused))
	group.Post("/crawl-intents/:id/resume", adminOnly, transitionIntent(deps, crawlstore.IntentStatusActive))
	group.Post("/crawl-intents/:id/archive", adminOnly, transitionIntent(deps, crawlstore.IntentStatusArchived))
	// Frequency edit + hard delete. Hard delete is distinct from
	// archive: the row goes away, not just hidden. Leads already
	// ingested by the intent are not cascaded — they are org-owned.
	group.Patch("/crawl-intents/:id/interval", adminOnly, setIntentInterval(deps))
	group.Delete("/crawl-intents/:id", adminOnly, deleteIntent(deps))
}
