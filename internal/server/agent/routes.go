package agent

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/store"
)

type Deps struct {
	DB       *store.Store
	Agent    *ai.Agent
	AIClass  *ai.MessageGenerator
	WSHub    *WSHub
	Notifier func(string)
}

type Handler struct {
	db       *store.Store
	agent    *ai.Agent
	aiClass  *ai.MessageGenerator
	wsHub    *WSHub
	notifier func(string)
}

func NewHandler(deps Deps) *Handler {
	return &Handler{
		db:       deps.DB,
		agent:    deps.Agent,
		aiClass:  deps.AIClass,
		wsHub:    deps.WSHub,
		notifier: deps.Notifier,
	}
}

// ConnectorRoutes registers token-authenticated Chrome Extension routes.
func ConnectorRoutes(group fiber.Router, deps Deps) {
	h := NewHandler(deps)

	group.Post("/connectors/heartbeat", h.agentAuth, h.agentHeartbeat)
	group.Post("/connectors/chrome-status", h.agentAuth, h.agentChromeStatus)
	group.Get("/connectors/browser-targets", h.agentAuth, h.agentBrowserTargets)
	group.Post("/connectors/screenshot", h.agentAuth, h.agentScreenshot)
	group.Post("/connectors/crawl-result", h.agentAuth, h.agentConnectorCrawlResult)
	group.Post("/connectors/crawl-progress", h.agentAuth, h.agentConnectorCrawlProgress)
	group.Get("/connectors/commands", h.agentAuth, h.agentConnectorCommands)
	group.Post("/connectors/commands/:id/done", h.agentAuth, h.agentConnectorCommandDone)
	group.Get("/connectors/outbox", h.agentAuth, h.agentGetOutbox)
	group.Post("/connectors/outbox/:id/sent", h.agentAuth, h.agentOutboxSent)
	group.Post("/connectors/outbox/:id/failed", h.agentAuth, h.agentOutboxFailed)

	agentGrp := group.Group("/agent", h.agentAuth)
	agentGrp.Post("/heartbeat", h.agentHeartbeat)
	agentGrp.Post("/chrome-status", h.agentChromeStatus)
	agentGrp.Get("/browser-targets", h.agentBrowserTargets)
	agentGrp.Post("/screenshot", h.agentScreenshot)
	agentGrp.Post("/crawl-result", h.agentConnectorCrawlResult)
	agentGrp.Post("/crawl-progress", h.agentConnectorCrawlProgress)
	agentGrp.Get("/commands", h.agentConnectorCommands)
	agentGrp.Post("/commands/:id/done", h.agentConnectorCommandDone)
	agentGrp.Get("/outbox", h.agentGetOutbox)
	agentGrp.Post("/outbox/:id/sent", h.agentOutboxSent)
	agentGrp.Post("/outbox/:id/failed", h.agentOutboxFailed)
	agentGrp.Get("/images", h.agentServeImage)
}

// AdminTokenRoutes registers JWT/admin-authenticated agent token management.
func AdminTokenRoutes(group fiber.Router, deps Deps) {
	h := NewHandler(deps)
	group.Post("/agent-tokens", h.agentCreateToken)
	group.Get("/agent-tokens", h.agentListTokens)
	group.Delete("/agent-tokens/:id", h.agentRevokeToken)
}

// DashboardRoutes registers tenant-authenticated AI prompt and outbox routes.
func DashboardRoutes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	h := NewHandler(deps)
	group.Post("/ai/prompt", h.aiPrompt)
	group.Get("/ai/history", h.aiHistory)
	group.Delete("/ai/history", h.aiDeleteHistory)
	group.Delete("/ai/history/:id", h.aiDeleteHistoryItem)

	group.Get("/outbox", h.getOutbox)
	group.Post("/outbox/draft", h.draftOutbound)
	group.Delete("/outbox/comments/all", adminOnly, h.deleteAllOutboundComments)
	group.Put("/outbox/:id/approve", h.approveOutbound)
	group.Put("/outbox/:id/reject", h.rejectOutbound)
	group.Put("/outbox/:id/content", h.editOutbound)
	group.Delete("/outbox/:id", h.deleteOutbound)
}
