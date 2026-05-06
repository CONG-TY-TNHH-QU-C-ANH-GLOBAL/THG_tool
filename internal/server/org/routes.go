package org

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
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
	group.Post("/users", tenantReady, adminOnly, h.createOrgUser)
	group.Get("/users", tenantReady, adminOnly, h.listUsers)
	group.Put("/users/:id", tenantReady, adminOnly, h.adminUpdateUser)
	group.Delete("/users/:id", tenantReady, adminOnly, h.adminDeleteUser)
	group.Get("/audit", tenantReady, adminOnly, h.getAuditLogs)
}

// Routes registers tenant and superadmin org endpoints.
func Routes(group fiber.Router, deps Deps, adminOnly fiber.Handler, founderOnly fiber.Handler) {
	h := &Handler{deps: deps}

	group.Get("/org", h.getMyOrg)
	group.Put("/org", adminOnly, h.updateOrg)
	group.Post("/org/assets/:kind", adminOnly, h.uploadOrgAsset)

	superAdminGrp := group.Group("/superadmin", founderOnly)
	superAdminGrp.Get("/orgs", h.listOrgs)
	superAdminGrp.Put("/orgs/:id", h.adminUpdateOrg)
	superAdminGrp.Delete("/orgs/:id", h.superAdminDeleteOrg)
	superAdminGrp.Get("/accounts", h.superAdminAccounts)
	superAdminGrp.Delete("/accounts/:id", h.superAdminDeleteAccount)
	superAdminGrp.Get("/users", h.superAdminUsers)
	superAdminGrp.Delete("/users/:id", h.superAdminDeleteUser)
	superAdminGrp.Get("/sessions", h.superAdminSessions)
	superAdminGrp.Delete("/sessions/:id", h.superAdminTerminateSession)
	superAdminGrp.Post("/query", h.superAdminQuery)
}
