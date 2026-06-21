// Package superadmin is the SaaS/platform control plane: cross-tenant,
// founder-only administration (org/account/user/session management, diagnostic
// reports, comment forensics). It is deliberately separate from the
// workspace/team-scoped org module — these endpoints operate across tenants and
// are gated by the platform founder role, not by per-workspace membership.
//
// Routes are registered by RegisterRoutes under the existing protected /api
// group, preserving the exact /api/superadmin/... effective paths and the
// founder-only middleware passed in by the composition root.
package superadmin

import "github.com/thg/scraper/internal/store"

// WorkspaceManager stops the live browser workspace for an account. It is the
// same narrow capability the org module needs; superadmin owns its own copy so
// the package does not depend on org just for this interface.
type WorkspaceManager interface {
	Stop(accountID int64)
}

// Deps are the dependencies the superadmin handlers need.
type Deps struct {
	DB        *store.Store
	Workspace WorkspaceManager
}

// Handler hosts the founder-only superadmin endpoints.
type Handler struct {
	deps Deps
}

// NewHandler builds a superadmin Handler.
func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}
