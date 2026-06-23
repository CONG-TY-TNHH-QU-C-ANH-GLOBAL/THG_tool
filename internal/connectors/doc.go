// Package connectors marks the target connector domain/infra boundary: the Chrome
// extension bridge, pairing, commands, screenshots, and readiness — the thing the
// connector driver pulls work through.
//
// Architecture role: DOMAIN/INFRASTRUCTURE — see MODULE_BOUNDARIES.md and
// CONNECTOR_STATE_MACHINE.md (the binding connector states + pull/CAS/lease model).
//
//   - Allowed imports (conceptual): models, its own store domain, ports, stdlib.
//   - Forbidden imports (conceptual): services/* workflows, drivers/*, internal/server
//     transport. MUST preserve pull / CAS / lease / human_required semantics; never
//     introduce server-push execution.
//
// SCAFFOLD ONLY (Phase A): boundary marker; no runtime logic lives here. Connector
// pieces currently live under internal/store/connectors, internal/browsergateway,
// internal/cdpclient, and local-connector-extension (see MODULE_OWNERSHIP.yml). Code
// migrates here only via a reviewed refactor — do not add or move runtime logic
// casually.
package connectors
