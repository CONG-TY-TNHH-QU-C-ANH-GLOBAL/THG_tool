// Package http marks the target inbound REST/SSE driver boundary: it translates
// HTTP requests/streams into application commands/queries and renders responses.
//
// Architecture role: DRIVERS/HTTP — see
// docs/architecture/ARCHITECTURE_STANDARD.md §3 and MODULE_BOUNDARIES.md.
//
//   - Allowed imports (conceptual): application command/query ports (consumer-owned),
//     models, stdlib.
//   - Forbidden imports (conceptual): DB repositories directly (internal/store/*)
//     except documented legacy exceptions, store/outbound internals, connector
//     internals, sibling business services. Owns no business logic and no direct
//     table writes beyond those exceptions.
//
// SCAFFOLD ONLY (Phase A): boundary marker; no runtime logic lives here. The HTTP
// driver currently lives under internal/server/* (see MODULE_OWNERSHIP.yml). Code
// migrates here only via a reviewed refactor — do not add or move runtime logic
// casually.
package http
