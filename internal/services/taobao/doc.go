// Package taobao marks the target application/workflow module for the future Taobao
// sourcing vertical: product sourcing and price-extraction workflows on the SHARED
// primitives (outbound, connectors, events, ai), same shape as services/facebook.
//
// Architecture role: SERVICES/TAOBAO — see MODULE_BOUNDARIES.md (services/taobao).
//
//   - Allowed imports (conceptual): shared primitives via ports, ai, models, stdlib.
//   - Forbidden imports (conceptual): internal/services/facebook and any sibling
//     service, Facebook-specific selectors or fburl, drivers/*, internal/server
//     transport.
//
// SCAFFOLD ONLY (Phase A): boundary marker; no workflow code has moved here. Today
// only a platform resolver stub exists (internal/platform/services/resolver). The
// rule is reserved and enforced (SERVICE_NO_SIBLING, warn-only) before any code
// lands — do not add runtime logic casually.
package taobao
