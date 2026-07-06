// Package reel is the Reel Studio workflow service: draft -> script ->
// approve -> render. It owns sequencing and state transitions only; all
// persistence goes through internal/store/reel, and video rendering goes
// through the consumer-owned VideoRenderer port (ports.go), whose only
// implementation in this PR is the deterministic FakeRenderer.
//
// Architecture role: SERVICES/REEL — see
// docs/architecture/decisions/ADR-reel-studio-platform-module.md.
//
//   - Allowed imports: internal/store/reel (via the *reel.Store handle
//     injected at construction), models, context/errors/fmt.
//   - Forbidden imports: internal/server transport, internal/store/outbound
//     or internal/store/coordination (post_reel/outbound publish is a later,
//     separate RED-zone PR), sibling services.
//
// PR-R2 (2026-07-06): workflow + fake renderer only. No HTTP, no frontend,
// no real provider adapters, no webhook, no outbound/action_ledger.
package reel
