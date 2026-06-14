// Package drivers groups the inbound adapters that translate the outside world into
// application commands/queries: copilot (NL prompt), http (REST/SSE), telegram
// (webhook), connector (extension endpoints). Drivers depend downward on the
// application layer only.
//
// Architecture role: DRIVERS (inbound adapters) — see
// docs/architecture/ARCHITECTURE_STANDARD.md §3 and MODULE_BOUNDARIES.md.
//
//   - Allowed imports (conceptual): application command ports (consumer-owned),
//     models, stdlib.
//   - Forbidden imports (conceptual): DB repositories directly, store/outbound
//     internals, connector internals, sibling business services. A driver hands a
//     typed command to the application layer; it never queues/claims/executes.
//
// SCAFFOLD ONLY (Phase A): boundary marker. Today the HTTP driver is
// internal/server, the connector driver is internal/server/agent, the telegram
// driver is internal/server/telegram + internal/telegram, and the copilot driver
// lives in internal/ai (agent*.go / intent_*.go). See MODULE_OWNERSHIP.yml.
package drivers
