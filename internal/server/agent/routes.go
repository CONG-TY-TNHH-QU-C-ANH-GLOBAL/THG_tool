package agent

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/drivers/copilot"
	"github.com/thg/scraper/internal/server/agent/account"
	"github.com/thg/scraper/internal/server/agent/crawlingest"
	"github.com/thg/scraper/internal/server/agent/outbox"
	"github.com/thg/scraper/internal/server/agent/presence"
	"github.com/thg/scraper/internal/server/agent/stream"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram/control"
)

type Deps struct {
	DB       *store.Store
	Agent    *copilot.Agent
	AIClass  func() *ai.MessageGenerator
	WSHub    *stream.WSHub
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
	notifier func(string)
	tgEvents *control.Service
	baseURL  string
}

func NewHandler(deps Deps) *Handler {
	return &Handler{
		db:       deps.DB,
		agent:    deps.Agent,
		aiClass:  deps.AIClass,
		notifier: deps.Notifier,
		tgEvents: deps.TgEvents,
		baseURL:  deps.BaseURL,
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
	agentGrp.Get("/images", h.agentServeImage)

	// Connector crawl-result ingestion lives in the crawlingest subpackage.
	crawlingest.RegisterRoutes(group, agentGrp, crawlingest.Deps{
		DB: deps.DB, AIClass: deps.AIClass, Notifier: deps.Notifier,
		TgEvents: deps.TgEvents, BaseURL: deps.BaseURL,
	}, h.agentAuth)
	// Outbound execution (outbox claim/sent/failed/pre-submit + comment reverify)
	// lives in the outbox subpackage — same effective paths + token auth; it owns
	// its own Handler and delegates the terminal step to finalize.
	outbox.RegisterConnectorRoutes(group, agentGrp, outbox.Deps{
		DB: deps.DB, Notifier: deps.Notifier, TgEvents: deps.TgEvents,
		BaseURL: deps.BaseURL, WSReady: deps.WSHub, RequireAccount: RequireAccountOwner,
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

	// Outbound dashboard (outbox draft/edit/delete + the byte-exact list response)
	// and comment human-verify/retry/metrics live in the outbox subpackage — same
	// effective paths + the same adminOnly gates. (/outbox/:id/approve+reject were
	// removed in the autonomous-first refactor; no human-approval gate remains.)
	outbox.RegisterDashboardRoutes(group, outbox.Deps{
		DB: deps.DB, Notifier: deps.Notifier, TgEvents: deps.TgEvents,
		BaseURL: deps.BaseURL, WSReady: deps.WSHub, RequireAccount: RequireAccountOwner,
	}, adminOnly)
}
