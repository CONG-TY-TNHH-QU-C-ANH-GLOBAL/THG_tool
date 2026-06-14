// Package outbound is the vertical-NEUTRAL outbound coordination spine: queue,
// dedup, claim (CAS/lease), transition, finalize, policy, and the append-only
// ledger + execution_attempts. It coordinates *actions*, not Facebook, so Taobao
// reuses it unchanged.
//
// Architecture role: OUTBOUND (domain) — see MODULE_BOUNDARIES.md (outbound) and
// CONNECTOR_STATE_MACHINE.md (action lifecycle).
//
//   - Allowed imports (conceptual): models, coordination ledger types (to write its
//     entries), events (publish transitions), store/dbutil, stdlib.
//   - Forbidden imports (conceptual): any service (services/facebook, …),
//     drivers/copilot, fburl, jobhandlers, internal/server. Outbound must stay
//     vertical-neutral.
//
// It OWNS the ActionExecutor port (consumer-owned): services implement it; outbound
// never imports a service to call one (PORTS_AND_ADAPTERS.md §2).
//
// SCAFFOLD ONLY (Phase A): boundary marker. The outbound store domain currently
// lives at internal/store/outbound (import-clean) + internal/store/coordination
// (append-only); the application orchestrator is cmd/scraper/outbound_actions.go.
// Splitting the neutral core from FB-specific resolution is roadmap Phase C/I.
package outbound
