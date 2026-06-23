// Package connector marks the target inbound Chrome-extension connector-driver
// boundary: heartbeat, outbox pull, and crawl/action result endpoints.
//
// Architecture role: DRIVERS/CONNECTOR — see
// docs/architecture/ARCHITECTURE_STANDARD.md §3, MODULE_BOUNDARIES.md, and
// CONNECTOR_STATE_MACHINE.md (the binding pull-based action lifecycle).
//
//   - Allowed imports (conceptual): application command ports (consumer-owned),
//     models, stdlib.
//   - Forbidden imports (conceptual): DB repositories directly, business workflows.
//     MUST preserve pull-based connector-outbox semantics: connectors PULL claimable
//     work; the server CLAIMS via CAS/lease. MUST NOT introduce server-push
//     execution.
//
// SCAFFOLD ONLY (Phase A): boundary marker; no runtime logic lives here. The
// connector driver currently lives under internal/server/agent (see
// MODULE_OWNERSHIP.yml). Code migrates here only via a reviewed refactor — do not
// add or move runtime logic casually.
package connector
