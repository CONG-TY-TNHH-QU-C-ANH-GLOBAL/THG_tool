// Package outbox owns the outbound-execution HTTP surface: the connector outbox
// claim/sent/failed/pre-submit callbacks (execution_id CAS), the async comment
// reverify endpoints, the operator outbox dashboard (draft/edit/delete + the
// byte-exact list response), and the comment human-verify/retry/metrics actions.
//
// Extracted from the flat internal/server/agent package via the self-Handler
// pattern: it holds its OWN Handler over a narrow dependency set and does NOT
// import agent, so there is no agent↔outbox cycle. It delegates the terminal
// finalize step to the finalize subpackage (outbox → finalize, one direction).
// The move preserves behavior exactly — the execution_id CAS, claim/lease,
// idempotency, action_ledger semantics, and the dashboard wire shape are all
// relocated verbatim; only their home package changes.
package outbox

import (
	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/agent/finalize"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram/control"
)

// outboxReadyNotifier is the consumer-owned port for the one live-stream push
// the dashboard makes when a draft becomes immediately executable. The agent
// *WSHub satisfies it; nil = no live notification.
type outboxReadyNotifier interface {
	NotifyOutboxReady(n int)
}

// accountOwnerGuard is the consumer-owned port for the shared execution-layer
// account-ownership check (agent.RequireAccountOwner). It stays in agent because
// other consumers (server/workspace) use it too; outbox receives it injected so
// it need not import agent. Signature matches agent.RequireAccountOwner exactly.
type accountOwnerGuard func(db *store.Store, c *fiber.Ctx, accountID, orgID, userID int64, role string) (*models.Account, error)

// Deps are the dependencies the outbound-execution surface needs. DB/Notifier/
// TgEvents/BaseURL feed the embedded finalize handler; WSReady is the dashboard
// live-push port.
type Deps struct {
	DB             *store.Store
	Notifier       func(string)
	TgEvents       *control.Service
	BaseURL        string
	WSReady        outboxReadyNotifier
	RequireAccount accountOwnerGuard
}

// Handler hosts the outbound-execution endpoints. Field names/types match the
// former agent.Handler fields so the relocated methods compile unchanged.
type Handler struct {
	db                  *store.Store
	notifier            func(string)
	finalize            *finalize.Handler
	wsReady             outboxReadyNotifier
	requireAccountOwner accountOwnerGuard
}

// NewHandler builds an outbox Handler, constructing its own finalize handler.
func NewHandler(deps Deps) *Handler {
	return &Handler{
		db:       deps.DB,
		notifier: deps.Notifier,
		finalize: finalize.NewHandler(finalize.Deps{
			DB: deps.DB, Notifier: deps.Notifier, TgEvents: deps.TgEvents, BaseURL: deps.BaseURL,
		}),
		wsReady:             deps.WSReady,
		requireAccountOwner: deps.RequireAccount,
	}
}

// RegisterConnectorRoutes wires the token-authenticated connector outbox + reverify
// endpoints (same paths/auth/order as the flat package). connectorGrp applies auth
// per-route; agentGrp already has auth at the group level.
func RegisterConnectorRoutes(connectorGrp, agentGrp fiber.Router, deps Deps, auth fiber.Handler) {
	h := NewHandler(deps)
	connectorGrp.Get("/connectors/outbox", auth, h.agentGetOutbox)
	connectorGrp.Post("/connectors/outbox/:id/sent", auth, h.agentOutboxSent)
	connectorGrp.Post("/connectors/outbox/:id/failed", auth, h.agentOutboxFailed)
	connectorGrp.Post("/connectors/outbox/:id/pre-submit-verify", auth, h.agentOutboxPreSubmitVerify) // Layer C (P1.3C)

	agentGrp.Get("/outbox", h.agentGetOutbox)
	agentGrp.Post("/outbox/:id/sent", h.agentOutboxSent)
	agentGrp.Post("/outbox/:id/failed", h.agentOutboxFailed)
	agentGrp.Post("/outbox/:id/pre-submit-verify", h.agentOutboxPreSubmitVerify) // Layer C (P1.3C)
	// Async comment reverify (spec: specs/domains/facebook-sales-intelligence/features/comment-automation/technical.md).
	agentGrp.Get("/reverify/claim", h.agentReverifyClaim)
	agentGrp.Post("/reverify/result", h.agentReverifyResult)
}

// RegisterDashboardRoutes wires the tenant/admin outbox dashboard + comment-verify
// endpoints (same paths/auth/order as the flat package).
func RegisterDashboardRoutes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	h := NewHandler(deps)
	group.Get("/outbox", h.getOutbox)
	group.Post("/outbox/draft", h.draftOutbound)
	group.Post("/comments/:id/human-verify", h.humanVerifyComment)
	group.Post("/comments/:id/retry", h.retryComment)
	group.Get("/comments/metrics", h.commentOutcomeMetrics)
	group.Delete("/outbox/comments/all", adminOnly, h.deleteAllOutboundComments)
	group.Delete("/outbox/posts/all", adminOnly, h.deleteAllOutboundPosts)
	group.Put("/outbox/:id/content", h.editOutbound)
	group.Delete("/outbox/:id", h.deleteOutbound)
	group.Post("/accounts/:id/clear-actor-block", adminOnly, h.clearActorBlock)
}
