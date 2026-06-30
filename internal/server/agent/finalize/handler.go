// Package finalize owns the outbound execution-finalization pipeline: the
// execution_id-gated CAS that turns a connector /sent or /failed callback into
// a terminal (state, outcome) on outbound_messages, the append-only action_ledger
// + execution_attempts writes, and the first-win side effects (risk signal,
// quota refund, inbox upsert, operator notification).
//
// Extracted from the flat internal/server/agent package: it holds its OWN
// Handler over a narrow dependency set and does NOT import agent, so there is no
// agent↔finalize import cycle. The agent package calls FinalizeOutbound through
// an injected *finalize.Handler. The move preserves behavior exactly — ledger
// semantics, the execution_id CAS, and the HTTP wire shape are relocated verbatim.
package finalize

import (
	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram/control"
)

// Deps are the dependencies the finalize pipeline needs (the subset of the agent
// Handler's deps these files used: db / notifier / tgEvents / baseURL).
type Deps struct {
	DB       *store.Store
	Notifier func(string)
	TgEvents *control.Service
	BaseURL  string
}

// Handler hosts FinalizeOutbound. Field names/types match the former agent.Handler
// fields so the relocated methods compile unchanged.
type Handler struct {
	db       *store.Store
	notifier func(string)
	tgEvents *control.Service
	baseURL  string
}

// NewHandler builds a finalize Handler from Deps.
func NewHandler(deps Deps) *Handler {
	return &Handler{
		db:       deps.DB,
		notifier: deps.Notifier,
		tgEvents: deps.TgEvents,
		baseURL:  deps.BaseURL,
	}
}

// FinalizeResolution is the HTTP-shaped result of a /sent or /failed callback.
// FinalizeOutbound builds one; the caller (agent outbox handler) writes it
// through. Centralising the shape keeps the terminal pathways (committed /
// idempotent replay / stale execution_id) easy to audit.
type FinalizeResolution struct {
	HTTPStatus int
	Body       fiber.Map
}

// Write emits the resolution as the HTTP response (wire shape unchanged).
func (f *FinalizeResolution) Write(c *fiber.Ctx) error {
	return c.Status(f.HTTPStatus).JSON(f.Body)
}
