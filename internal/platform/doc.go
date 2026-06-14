// Package platform is the neutral, multi-service shell every vertical plugs into:
// the service registry, workspace/org tenancy root, and composition of the service
// list. It hosts services; it must not know their internals.
//
// Architecture role: PLATFORM (see docs/architecture/ARCHITECTURE_STANDARD.md §3-4
// and MODULE_BOUNDARIES.md).
//
//   - Allowed imports (conceptual): models, store users/org accessors, service
//     contracts it hosts, stdlib.
//   - Forbidden imports (conceptual): any business service (services/facebook,
//     services/taobao, …), ai intelligence/generators, jobhandlers, leadingest,
//     drivers/*. Do not encode "if service == facebook then …" here.
//
// SCAFFOLD ONLY (Phase A): this doc.go marks the target module boundary. Runtime
// code has NOT moved here yet — see docs/architecture/REFACTOR_ROADMAP.md and
// MODULE_OWNERSHIP.yml for the migration phase. The existing platform service
// registry currently lives at internal/platform/services.
package platform
