// Package copilot is the natural-language inbound driver: it turns a user prompt
// into a normalized intent and dispatches a typed application command (e.g.
// comment_single_post, crawl_group). It owns routing, typo/multilingual NLU, and
// ask-back — but no business rules and no data access.
//
// Architecture role: DRIVERS/COPILOT — see MODULE_BOUNDARIES.md (drivers/copilot)
// and PORTS_AND_ADAPTERS.md §3.
//
//   - Allowed imports (conceptual): the ai intent layer (pure), fburl (URL trust),
//     the application CommandBus port it owns, models, stdlib.
//   - Forbidden imports (conceptual): DB repositories directly (internal/store/*),
//     internal/server, store/outbound, connector internals. The driver dispatches a
//     command; it does not queue/claim outbound itself.
//
// SCAFFOLD ONLY (Phase A): boundary marker. The copilot driver currently lives in
// internal/ai (agent*.go, intent_*.go, brain*.go) and still holds *store.Store +
// an untyped ActionHandler — the documented gap (CURRENT_CODE_AUDIT.md §2). It
// migrates here with a typed CommandBus port in roadmap Phase D/G.
package copilot
