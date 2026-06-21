// Package presence owns the operator-facing connector presence & overview
// boards: the per-account connector presence board (GET /connectors/status) and
// the admin connector overview (GET /admin/connectors/overview). Both are
// read-only projections of connector + account operational state (online,
// readiness, version, identity match) — they grant NO device control and never
// serialize credentials/cookies/proxy/session data.
//
// Extracted from the agent package so this read-only "is my account reachable"
// concern has its own ownership boundary. It depends only on a narrow read-only
// store handle; it does not import the agent package.
package presence

import (
	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/store"
)

// Deps are the dependencies the presence boards need (read-only store access).
type Deps struct {
	DB *store.Store
}

// Handler hosts the connector presence/overview endpoints.
type Handler struct {
	db *store.Store
}

// NewHandler builds a presence Handler.
func NewHandler(deps Deps) *Handler {
	return &Handler{db: deps.DB}
}

// RegisterRoutes mounts the presence boards on the given (already
// tenant-authenticated) group, preserving the exact paths and middleware these
// handlers had inside agent.DashboardRoutes: /connectors/status is tenant-auth,
// /admin/connectors/overview is additionally gated by adminOnly.
func RegisterRoutes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	h := NewHandler(deps)
	group.Get("/connectors/status", h.connectorStatus)
	group.Get("/admin/connectors/overview", adminOnly, h.connectorOverview)
}
