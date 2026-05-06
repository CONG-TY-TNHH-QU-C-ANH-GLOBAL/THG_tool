package crawl

import "github.com/gofiber/fiber/v2"

// Routes registers crawl intent endpoints.
func Routes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	group.Get("/crawl-intents", listIntents(deps))
	group.Put("/crawl-intents/:id/enabled", adminOnly, setIntentEnabled(deps))
}
