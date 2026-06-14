// Package services is the parent of the per-vertical application/workflow modules
// (services/facebook, services/taobao, services/1688). Each service orchestrates
// domains for one vertical on the SHARED primitives (outbound, connectors, events,
// ai); services never import each other.
//
// Architecture role: APPLICATION / WORKFLOWS — see
// docs/architecture/ARCHITECTURE_STANDARD.md §3-4 and MODULE_BOUNDARIES.md.
//
//   - Allowed imports (conceptual): domain ports + domain modules, shared
//     primitives via ports, models, stdlib.
//   - Forbidden imports (conceptual): drivers/* (a service must not depend on the
//     driver that calls it), sibling services, transport (internal/server).
//
// NOTE: distinct from internal/platform/services, which is the platform service
// REGISTRY (resolver/status of the multi-service shell). This package is the home
// of the actual per-vertical WORKFLOWS.
//
// SCAFFOLD ONLY (Phase A): boundary marker; no workflow code has moved here yet.
package services
