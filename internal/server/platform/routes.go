// Package platform exposes the HTTP layer for the platform service registry.
// It composes adapters (storage -> domain) and resolvers (domain -> contract);
// no business branching lives here. See project_pr2_backend_mandates.md.
package platform

import (
	"github.com/gofiber/fiber/v2"
	platformsvc "github.com/thg/scraper/internal/platform/services"
	"github.com/thg/scraper/internal/store"
)

// Deps holds dependencies for the platform service handlers.
type Deps struct {
	DB *store.Store
}

// Routes registers the platform service registry endpoints under an
// already-authenticated group.
func Routes(group fiber.Router, deps Deps) {
	h := &Handler{
		deps:     deps,
		registry: platformsvc.DefaultRegistry(),
	}
	group.Get("/platform/services", h.listServices)
}
