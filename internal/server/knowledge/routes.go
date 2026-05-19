// Package knowledge exposes the HTTP surface for the Workspace
// Knowledge OS — register / list / update / delete / sync data
// sources. The package is a thin HTTP shell over the existing
// internal/workspace_knowledge/ingestion runtime: dispatcher,
// registry, store-backed asset writer.
//
// The dispatcher is constructed once at boot in router.go and shared
// across requests; this package never instantiates one itself. Per
// the existing convention (see internal/server/autoflow), routes
// gate write operations on the adminOnly middleware and read
// operations on the standard auth chain.
package knowledge

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
)

// Deps is the dependency bundle for this package's handlers. Deps
// fields are pointers so a stubbed store / dispatcher can be passed
// in tests without standing up the whole server.
type Deps struct {
	DB         *store.Store
	Dispatcher *ingestion.Dispatcher
}

// Routes registers the /knowledge subtree under the supplied group.
// The group is expected to already carry the org-scoping middleware
// chain (c.Locals("org_id") populated). adminOnly gates writes.
func Routes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	h := &handler{deps: deps}

	group.Get("/knowledge/sources", h.listSources)
	group.Post("/knowledge/sources", adminOnly, h.createSource)
	group.Patch("/knowledge/sources/:id", adminOnly, h.updateSource)
	group.Delete("/knowledge/sources/:id", adminOnly, h.deleteSource)
	// Sync is admin-only because a forced sync touches an upstream
	// system on the org's behalf — non-admins should never trigger
	// outbound traffic against a tenant-configured backend.
	group.Post("/knowledge/sources/:id/sync", adminOnly, h.syncSource)
}

type handler struct{ deps Deps }
