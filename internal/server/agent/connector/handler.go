// Package connector owns the connector-lifecycle HTTP surface: the agent-token
// authenticated Chrome-Extension callbacks (heartbeat / chrome-status /
// browser-targets / screenshot / commands), connector self-disconnect, agent
// token CRUD, plus the dashboard-authenticated connector management
// (LocalConnectorHandler: pairing-code minting + remote input commands).
//
// Extracted from the flat internal/server/agent package: it holds its own
// handlers over narrow dependency sets and does NOT import agent, so there is no
// agent↔connector import cycle. The parent agent package composes/delegates only.
// The move preserves behavior exactly — connector pairing/token CAS, heartbeat
// presence upsert, and the input-command flow are relocated verbatim; shared
// helpers (AgentTokenFingerprint / ClampPresenceFields) come from server middleware.
package connector

import (
	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram/control"
)

// Deps are the dependencies the agent-token connector callbacks need.
type Deps struct {
	DB       *store.Store
	TgEvents *control.Service
}

// Handler hosts the agent-token-authenticated connector callbacks + token CRUD.
// Field names/types match the former agent.Handler fields so the relocated
// methods compile unchanged.
type Handler struct {
	db       *store.Store
	tgEvents *control.Service
}

// NewHandler builds the connector callback Handler from Deps.
func NewHandler(deps Deps) *Handler {
	return &Handler{db: deps.DB, tgEvents: deps.TgEvents}
}

// RegisterCallbackRoutes wires the token-authenticated Chrome-Extension connector
// callbacks onto the parent route groups, preserving the exact paths, auth and
// ordering the flat agent package used. connectorGrp applies auth per-route;
// agentGrp already has auth applied at the group level.
func RegisterCallbackRoutes(connectorGrp, agentGrp fiber.Router, deps Deps, auth fiber.Handler) {
	h := NewHandler(deps)
	connectorGrp.Post("/connectors/heartbeat", auth, h.agentHeartbeat)
	connectorGrp.Post("/connectors/chrome-status", auth, h.agentChromeStatus)
	connectorGrp.Get("/connectors/browser-targets", auth, h.agentBrowserTargets)
	connectorGrp.Post("/connectors/screenshot", auth, h.agentScreenshot)
	connectorGrp.Get("/connectors/commands", auth, h.agentConnectorCommands)
	connectorGrp.Post("/connectors/commands/:id/done", auth, h.agentConnectorCommandDone)
	// Forget Device: the extension releases its own binding before wiping local
	// storage, so the Chrome profile becomes re-pairable by anyone.
	connectorGrp.Post("/connectors/self/disconnect", auth, h.agentSelfDisconnect)

	agentGrp.Post("/heartbeat", h.agentHeartbeat)
	agentGrp.Post("/chrome-status", h.agentChromeStatus)
	agentGrp.Get("/browser-targets", h.agentBrowserTargets)
	agentGrp.Post("/screenshot", h.agentScreenshot)
	agentGrp.Get("/commands", h.agentConnectorCommands)
	agentGrp.Post("/commands/:id/done", h.agentConnectorCommandDone)
}

// RegisterAdminTokenRoutes wires the JWT/admin-authenticated agent token CRUD.
func RegisterAdminTokenRoutes(group fiber.Router, deps Deps) {
	h := NewHandler(deps)
	group.Post("/agent-tokens", h.agentCreateToken)
	group.Get("/agent-tokens", h.agentListTokens)
	group.Delete("/agent-tokens/:id", h.agentRevokeToken)
}
