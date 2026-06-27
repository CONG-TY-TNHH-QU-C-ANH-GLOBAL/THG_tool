---
id: ARCHSV2
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
blocked_on: human-boundary-decision
---

# ARCHSV2 — Extract internal/server/agent/finalize subpackage

## Goal
The agent package is a 49-file flat package with a 13×crawl_/5×finalize_/4×outbox_ prefix smell. Move the outbound finalization cluster (finalize_outbound.go, finalize_side_effects.go, finalize_helpers.go) into a bounded `finalize/` subpackage with a small facade.

## Component / domain
internal/server/agent finalization state-machine + side effects.

## Files likely involved
finalize_outbound.go (190), finalize_side_effects.go (191), finalize_helpers.go (186) → internal/server/agent/finalize/; caller outbox_agent.go updates to the facade.

## Dependencies
None.

## Feasibility (VERIFIED 2026-06-27 — move-only is NOT possible; reclassified YELLOW→RED)
The "move-only, small facade" framing does not hold. The finalize cluster is not a
set of pure functions — it is a Handler-and-transport-coupled state machine:

- `finalize_outbound.go` defines `type outboundFinalizer struct { h *Handler; ... }`
  — the struct's first field is `*Handler`. All 14 `outboundFinalizer` methods reach
  back through `f.h.db`, `f.h.notifier`, `f.h.tgEvents`, `f.h.baseURL`, `f.h.orgName()`,
  `f.h.agentName()`.
- `finalizeOutbound` is a method on `*Handler` and takes `c *fiber.Ctx`;
  `finalizeResolution.write(c *fiber.Ctx)` is HTTP transport.
- Caller: only `outbox_agent.go` (`h.finalizeOutbound` at 2 sites).
- Store access is via exported store methods (no unexported ledger internals) — but
  that is the only GREEN aspect.

**Import-cycle blocker:** moving the cluster to `finalize/` makes `finalize` import
`agent` (for the `Handler` type its struct field needs) while `agent` imports
`finalize` (orchestration) → an `agent ↔ finalize` cycle. The ONLY way to break it
is to replace `h *Handler` with a dependency-injection port carrying the narrow deps
— a broad abstraction threaded through the CAS-adjacent `FinalizeOutboundAttempt`
path, and it would also pull `fiber` transport into the domain subpackage. The item's
own risk note pre-authorizes STOP for exactly this; CLAUDE.md stop conditions name
"a safe move needs a broad port/abstraction" as a stop.

Note: the 14 methods are ALREADY package-private (Go visibility), so the move buys no
encapsulation — only a new cross-package seam through finalization/ledger code.

## BLOCKED — E3 boundary / RED-adjacent (awaiting founder)
This is not a `/thg-next` mechanical move. Options:

- **Option A — defer (recommended):** leave the cluster in `internal/server/agent/`.
  The "5×finalize_ prefix" is a cosmetic smell; ARCHSV1 already trimmed the package,
  and the finalizer is correctly package-private today. Lowest risk, zero churn.
- **Option B — deliberate DI-port refactor (separate behavior-risk PR, NOT move-only):**
  decouple `outboundFinalizer` from `*Handler`/`fiber` via an injected dependency
  struct, then move. Requires founder approval, idempotency-replay tests guarding the
  CAS gate, and staged PRs (additive port → move) — out of scope for an autopilot move.
- **Option C — re-scope to a GREEN slice:** move only the 4 pure free functions
  (notificationDetail, agentEventType, persistEvidenceScreenshot, proofToEvidence) into
  a finalize helper file. Marginal value; does not achieve the item's stated goal and
  splits the unit. Not recommended on its own.

## Validation
go test ./internal/server/agent/... ; go vet ; ai_validate.sh

## Done criteria
finalize/ subpackage with package doc + facade; callers updated; no import cycle;
idempotency tests green; move-only diff. NOTE: "move-only diff / no import cycle" is
NOT achievable as written — see Feasibility; the done criteria must be re-stated per
the chosen option before any implementation.
