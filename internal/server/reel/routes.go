// Package reel exposes the internal Reel Studio workflow
// (internal/services/reel) over org-scoped HTTP routes, behind the
// REEL_STUDIO_ENABLED feature flag. Backend + fake-renderer only: no real
// provider, no webhook, no outbound. org_id/created_by are derived from the
// authenticated JWT context (never the request body); the routes mount on the
// protected group, so tenant isolation is the auth middleware's guarantee,
// not reimplemented here.
package reel

import (
	"github.com/gofiber/fiber/v2"

	reelsvc "github.com/thg/scraper/internal/services/reel"
	"github.com/thg/scraper/internal/store"
)

// Deps holds dependencies for the Reel Studio HTTP handlers.
type Deps struct {
	DB      *store.Store
	Enabled bool // REEL_STUDIO_ENABLED — when false the routes are not mounted.
}

type handler struct {
	svc *reelsvc.Service
}

// Routes mounts the Reel Studio workflow endpoints under /reels. When the
// feature flag is off nothing is registered, so every path 404s — the
// backend stays an API-only surface with no half-exposed reel routes.
func Routes(group fiber.Router, deps Deps) {
	if !deps.Enabled {
		return
	}
	h := &handler{svc: reelsvc.NewService(deps.DB.Reel(), reelsvc.FakeRenderer{})}

	g := group.Group("/reels")
	g.Post("/", h.createDraft)
	g.Post("/:reel_id/script", h.generateScript)
	g.Post("/:reel_id/approve", h.approveLatest)
	g.Post("/:reel_id/render/fake", h.renderFake)
}
