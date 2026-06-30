package agent

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/drivers/copilot"
	"github.com/thg/scraper/internal/server/agent/account"
	"github.com/thg/scraper/internal/server/agent/crawlingest"
	"github.com/thg/scraper/internal/server/agent/finalize"
	"github.com/thg/scraper/internal/server/agent/presence"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram/control"
)

type Deps struct {
	DB       *store.Store
	Agent    *copilot.Agent
	AIClass  func() *ai.MessageGenerator
	WSHub    *WSHub
	Notifier func(string)
	// TgEvents emits per-org Telegram CHANNEL notifications (lead_created, comment_*). Optional;
	// nil = no channel notifications. Shared with the integrations/webhook control service.
	TgEvents *control.Service
	// BaseURL is the public app URL used to build dashboard/post links in notifications.
	BaseURL string
}

type Handler struct {
	db       *store.Store
	agent    *copilot.Agent
	aiClass  func() *ai.MessageGenerator
	wsHub    *WSHub
	notifier func(string)
	tgEvents *control.Service
	baseURL  string
	finalize *finalize.Handler
}

func NewHandler(deps Deps) *Handler {
	return &Handler{
		db:       deps.DB,
		agent:    deps.Agent,
		aiClass:  deps.AIClass,
		wsHub:    deps.WSHub,
		notifier: deps.Notifier,
		tgEvents: deps.TgEvents,
		baseURL:  deps.BaseURL,
		finalize: finalize.NewHandler(finalize.Deps{
			DB: deps.DB, Notifier: deps.Notifier, TgEvents: deps.TgEvents, BaseURL: deps.BaseURL,
		}),
	}
}

// ConnectorRoutes registers token-authenticated Chrome Extension routes.
func ConnectorRoutes(group fiber.Router, deps Deps) {
	h := NewHandler(deps)

	group.Post("/connectors/heartbeat", h.agentAuth, h.agentHeartbeat)
	group.Post("/connectors/chrome-status", h.agentAuth, h.agentChromeStatus)
	group.Get("/connectors/browser-targets", h.agentAuth, h.agentBrowserTargets)
	group.Post("/connectors/screenshot", h.agentAuth, h.agentScreenshot)
	group.Get("/connectors/commands", h.agentAuth, h.agentConnectorCommands)
	group.Post("/connectors/commands/:id/done", h.agentAuth, h.agentConnectorCommandDone)
	group.Get("/connectors/outbox", h.agentAuth, h.agentGetOutbox)
	group.Post("/connectors/outbox/:id/sent", h.agentAuth, h.agentOutboxSent)
	group.Post("/connectors/outbox/:id/failed", h.agentAuth, h.agentOutboxFailed)
	group.Post("/connectors/outbox/:id/pre-submit-verify", h.agentAuth, h.agentOutboxPreSubmitVerify) // Layer C (P1.3C)
	// Forget Device: the extension releases its own binding before wiping
	// local storage, so the Chrome profile becomes re-pairable by anyone.
	group.Post("/connectors/self/disconnect", h.agentAuth, h.agentSelfDisconnect)

	agentGrp := group.Group("/agent", h.agentAuth)
	agentGrp.Post("/heartbeat", h.agentHeartbeat)
	agentGrp.Post("/chrome-status", h.agentChromeStatus)
	agentGrp.Get("/browser-targets", h.agentBrowserTargets)
	agentGrp.Post("/screenshot", h.agentScreenshot)
	agentGrp.Get("/commands", h.agentConnectorCommands)
	agentGrp.Post("/commands/:id/done", h.agentConnectorCommandDone)
	agentGrp.Get("/outbox", h.agentGetOutbox)
	agentGrp.Post("/outbox/:id/sent", h.agentOutboxSent)
	agentGrp.Post("/outbox/:id/failed", h.agentOutboxFailed)
	agentGrp.Post("/outbox/:id/pre-submit-verify", h.agentOutboxPreSubmitVerify) // Layer C (P1.3C)
	// Async comment reverify (spec: specs/COMMENT_ASYNC_REVERIFY.md).
	agentGrp.Get("/reverify/claim", h.agentReverifyClaim)
	agentGrp.Post("/reverify/result", h.agentReverifyResult)
	agentGrp.Get("/images", h.agentServeImage)

	// Connector crawl-result ingestion (crawl-result/crawl-progress on both the
	// /connectors and /agent groups) lives in the crawlingest subpackage — same
	// effective paths + token auth; the cluster owns its own Handler.
	crawlingest.RegisterRoutes(group, agentGrp, crawlingest.Deps{
		DB: deps.DB, AIClass: deps.AIClass, Notifier: deps.Notifier,
		TgEvents: deps.TgEvents, BaseURL: deps.BaseURL,
	}, h.agentAuth)
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
	// Connector presence board (tenant) + admin connector overview live in the
	// presence subpackage — read-only connector/account operational views.
	// Same effective paths (/connectors/status, /admin/connectors/overview) and
	// the same adminOnly gate on the overview.
	presence.RegisterRoutes(group, presence.Deps{DB: deps.DB}, adminOnly)
	// PR-D readiness matrix (per-account, per-capability "can run + why not")
	// lives in the account subpackage — same effective path /accounts/readiness.
	account.RegisterRoutes(group, account.Deps{DB: deps.DB})
	group.Delete("/ai/history", h.aiDeleteHistory)
	group.Delete("/ai/history/:id", h.aiDeleteHistoryItem)

	group.Get("/outbox", h.getOutbox)
	group.Post("/outbox/draft", h.draftOutbound)
	// Manual human verification + retry + outcome metrics (spec: COMMENT_ASYNC_REVERIFY.md).
	group.Post("/comments/:id/human-verify", h.humanVerifyComment)
	group.Post("/comments/:id/retry", h.retryComment)
	group.Get("/comments/metrics", h.commentOutcomeMetrics)
	group.Delete("/outbox/comments/all", adminOnly, h.deleteAllOutboundComments)
	group.Delete("/outbox/posts/all", adminOnly, h.deleteAllOutboundPosts)
	// /outbox/:id/approve and /outbox/:id/reject were removed in the
	// autonomous-first refactor (May-2026). The system no longer has
	// a human-approval gate — every queued outbound is planned and
	// executes when an account is available.
	group.Put("/outbox/:id/content", h.editOutbound)
	group.Delete("/outbox/:id", h.deleteOutbound)

	// Verified Actor (P1b): operator override to lift an actor-mismatch block
	// on an account so it can auto-execute again. Admin-only.
	group.Post("/accounts/:id/clear-actor-block", adminOnly, h.clearActorBlock)
}
