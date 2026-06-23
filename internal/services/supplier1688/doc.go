// Package supplier1688 marks the target application/workflow module for the future
// 1688 sourcing vertical (the "1688" service; named supplier1688 because a Go
// package identifier cannot begin with a digit). Product sourcing and
// price-extraction workflows on the SHARED primitives, same shape as
// services/facebook.
//
// Architecture role: SERVICES/1688 — see MODULE_BOUNDARIES.md (services/1688).
//
//   - Allowed imports (conceptual): shared primitives via ports, ai, models, stdlib.
//   - Forbidden imports (conceptual): internal/services/facebook and any sibling
//     service, Facebook-specific selectors or fburl, drivers/*, internal/server
//     transport.
//
// SCAFFOLD ONLY (Phase A): boundary marker; no workflow code has moved here. Today
// only a platform resolver stub exists (internal/platform/services/resolver,
// alibaba1688.go). The rule is reserved and enforced (SERVICE_NO_SIBLING,
// warn-only) before any code lands — do not add runtime logic casually.
package supplier1688
