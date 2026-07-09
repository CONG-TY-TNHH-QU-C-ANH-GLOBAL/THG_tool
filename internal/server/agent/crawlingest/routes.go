// Package crawlingest owns the connector crawl-result ingestion pipeline:
// the Chrome-extension callbacks that report crawl results/progress
// (POST /connectors/crawl-result, /crawl-progress and their /agent/* aliases),
// the lead-ingestion + direct-post intake resolution they drive, and the
// direct-post CAS that gates a direct-post import outcome.
//
// Extracted from the flat internal/server/agent package (component-structure
// boundary): it holds its OWN Handler over a narrow dependency set and does NOT
// import the agent package, so there is no agent↔crawlingest import cycle. The
// move preserves behavior exactly — the direct-post CAS and all ingestion logic
// are relocated verbatim; only their home package changes.
package crawlingest

import (
	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/session/accountsafety"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram/control"
)

// Deps are the dependencies the crawl-ingest pipeline needs. Mirrors the subset
// of the agent Handler's deps this cluster used (db / aiClass / notifier /
// tgEvents / baseURL) — no connector/WS/agent coupling.
type Deps struct {
	DB       *store.Store
	AIClass  func() *ai.MessageGenerator
	Notifier func(string)
	TgEvents *control.Service
	BaseURL  string
	// AccountSafety is the process-local coordinator shared with the crawl
	// scheduler (PR-C4): every terminal crawl result reports its exit_reason so
	// the machine slot frees immediately and risk exits park the account.
	// Optional; nil = no result feedback.
	AccountSafety *accountsafety.Coordinator
}

// Handler hosts the crawl-result ingestion endpoints. Field names/types match
// the former agent.Handler fields so the relocated methods compile unchanged.
type Handler struct {
	db            *store.Store
	aiClass       func() *ai.MessageGenerator
	notifier      func(string)
	tgEvents      *control.Service
	baseURL       string
	accountSafety *accountsafety.Coordinator
}

// NewHandler builds a crawl-ingest Handler from Deps.
func NewHandler(deps Deps) *Handler {
	return &Handler{
		db:            deps.DB,
		aiClass:       deps.AIClass,
		notifier:      deps.Notifier,
		tgEvents:      deps.TgEvents,
		baseURL:       deps.BaseURL,
		accountSafety: deps.AccountSafety,
	}
}

// RegisterRoutes wires the connector crawl-result endpoints onto the parent's
// route groups, preserving the exact paths, auth middleware and ordering the
// flat agent package used. connectorGrp is the token-authenticated /connectors
// group (auth applied per-route); agentGrp is the /agent group that already has
// auth applied at the group level.
func RegisterRoutes(connectorGrp, agentGrp fiber.Router, deps Deps, auth fiber.Handler) {
	h := NewHandler(deps)
	connectorGrp.Post("/connectors/crawl-result", auth, h.agentConnectorCrawlResult)
	connectorGrp.Post("/connectors/crawl-progress", auth, h.agentConnectorCrawlProgress)
	agentGrp.Post("/crawl-result", h.agentConnectorCrawlResult)
	agentGrp.Post("/crawl-progress", h.agentConnectorCrawlProgress)
}
