package org

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
)

// Shared org user route path segments. Defined once so the literal has a
// single source.
const (
	routeUsers    = "/users"
	routeUserByID = "/users/:id"
)

type WorkspaceManager interface {
	Stop(accountID int64)
}

// Deps holds dependencies needed by organization handlers.
type Deps struct {
	DB        *store.Store
	JWTSecret string
	Workspace WorkspaceManager
}

type Handler struct {
	deps Deps
}

// PublicRoutes registers public org endpoints.
func PublicRoutes(api fiber.Router, deps Deps, regLimiter fiber.Handler) {
	h := &Handler{deps: deps}
	api.Post("/register", regLimiter, h.registerOrg)
	api.Get("/public/org-assets/:orgID/:kind", h.serveOrgAsset)
}

// AuthAdminRoutes registers auth-scoped admin org user endpoints.
func AuthAdminRoutes(group fiber.Router, deps Deps, tenantReady, adminOnly fiber.Handler) {
	h := &Handler{deps: deps}
	group.Post(routeUsers, tenantReady, adminOnly, h.createOrgUser)
	group.Get(routeUsers, tenantReady, adminOnly, h.listUsers)
	group.Put(routeUserByID, tenantReady, adminOnly, h.adminUpdateUser)
	group.Delete(routeUserByID, tenantReady, adminOnly, h.adminDeleteUser)
	group.Get("/audit", tenantReady, adminOnly, h.getAuditLogs)
}

// Routes registers tenant-scoped org endpoints. Founder-only superadmin
// endpoints live in the internal/server/superadmin module and are registered
// separately at the composition root.
func Routes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	h := &Handler{deps: deps}

	group.Get("/org", h.getMyOrg)
	group.Put("/org", adminOnly, h.updateOrg)
	group.Post("/org/assets/:kind", adminOnly, h.uploadOrgAsset)
	group.Get("/org/business-profile", h.getBusinessProfile)
	group.Put("/org/business-profile", adminOnly, h.updateBusinessProfile)
	// Company Identity form (brand/website/contact/CTA used by grounded comments).
	group.Get("/org/company-identity", h.getCompanyIdentity)
	group.Put("/org/company-identity", adminOnly, h.updateCompanyIdentity)

	// Workspace Knowledge OS — Operator Replay surface. Read-only;
	// tenant-scoped via c.Locals("org_id"). See
	// specs/WORKSPACE_KNOWLEDGE_OS.md.
	group.Get("/org/knowledge/events", h.listKnowledgeEvents)
	group.Get("/org/knowledge/events/:retrieval_id", h.getKnowledgeEvent)
	group.Get("/org/knowledge/sources/:source_id/syncs", h.listSourceSyncs)
	group.Get("/org/knowledge/stats", h.getKnowledgeStats)
	group.Get("/org/knowledge/soak", h.getKnowledgeSoak)
}
