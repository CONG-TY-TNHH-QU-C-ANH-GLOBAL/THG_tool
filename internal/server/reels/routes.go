// Package reels is the THIN HTTP transport for the reel workflow. Handlers parse the
// request, call the reel application service, and marshal JSON — no business logic, no
// render/spend decisions live here (those belong to internal/services/reel). The render
// webhook is PUBLIC (a render provider cannot send a JWT); authenticity is an HMAC check.
package reels

import (
	"github.com/gofiber/fiber/v2"
	reelsvc "github.com/thg/scraper/internal/services/reel"
)

// Deps wires the reel service + the webhook HMAC secret.
type Deps struct {
	Service       *reelsvc.Service
	WebhookSecret string
}

// Routes registers the authenticated reel endpoints on the protected (JWT) group.
func Routes(group fiber.Router, deps Deps) {
	group.Post("/reels", createReel(deps))
	group.Get("/reels", listReels(deps))
	group.Get("/reels/:id", getReel(deps))
	group.Get("/reels/:id/video", serveVideo(deps))
	group.Patch("/reels/:id/script", updateScript(deps))
	group.Post("/reels/:id/approve", approveReel(deps))
	group.Post("/reels/:id/publish", publishReel(deps))
}

// WebhookRoutes registers the PUBLIC render webhook on the unauthenticated /api group.
func WebhookRoutes(api fiber.Router, deps Deps) {
	api.Post("/reel/webhook/render", renderWebhook(deps))
}
