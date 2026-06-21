// Package account owns the tenant-facing account readiness/executability slice:
// the per-account, per-capability readiness matrix (GET /accounts/readiness) and
// the requester-scoped executability resolver behind it. Read-only projections
// that reuse the same connector + behaviour-cap evaluators the execution gate
// uses, so the UI can never disagree with what execution will actually do.
//
// Extracted from the agent package to give this concern its own ownership
// boundary. It depends only on a narrow read-only store handle and does not
// import the agent package.
package account

import (
	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/store"
)

// Deps are the dependencies the account readiness endpoints need.
type Deps struct {
	DB *store.Store
}

// Handler hosts the account readiness endpoints.
type Handler struct {
	db *store.Store
}

// NewHandler builds an account Handler.
func NewHandler(deps Deps) *Handler {
	return &Handler{db: deps.DB}
}

// RegisterRoutes mounts the account readiness endpoints on the given (already
// tenant-authenticated) group, preserving the exact path the handler had inside
// agent.DashboardRoutes.
func RegisterRoutes(group fiber.Router, deps Deps) {
	h := NewHandler(deps)
	group.Get("/accounts/readiness", h.accountReadiness)
}
