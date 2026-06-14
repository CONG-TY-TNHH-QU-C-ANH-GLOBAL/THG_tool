// Package ai is PURE intelligence: classify leads, generate/repair comment & inbox
// copy, score, and decide intent shape. It is deterministic, IO-free, and
// platform-neutral. It must not invent business facts — missing grounding returns a
// typed knowledge_gap, never a hallucination.
//
// Architecture role: AI (pure domain) — see MODULE_BOUNDARIES.md (ai) and
// CURRENT_CODE_AUDIT.md.
//
//   - Allowed imports (conceptual): internal/models + stdlib ONLY.
//   - Forbidden imports (conceptual): store, server, connector/browsergateway,
//     outbound, jobs, any services/* workflow, platform workflow.
//
// IMPORTANT — this package currently MIXES two logical modules:
//   - the PURE intelligence (internal/ai/comment, generators) — already
//     import-clean (models-only); and
//   - the COPILOT DRIVER (agent*.go, intent_*.go, brain*.go) — an inbound adapter
//     that today imports *store.Store. That driver belongs in
//     internal/drivers/copilot and is the documented gap surfaced by
//     scripts/check_import_boundaries.sh (COPILOT_NO_DIRECT_REPO).
//
// SCAFFOLD/STATUS (Phase A): no code moved. The driver/intelligence split is roadmap
// Phase B (pure AI boundary) + Phase G (copilot cleanup). See MODULE_OWNERSHIP.yml.
package ai
