---
id: ARCHCM2b
status: BLOCKED
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM1]
parallel_safe: false
branch: "audit/archcm2b-target-rescope"
pr_url: ""
blocked_on: comment-usecase-home-decision
boundary_target: blocked-decision
audit_status: COMPLETE
---

# ARCHCM2b — Move comment_reasoning out of cmd (target was WRONG — re-scoped)

## Goal
Move the comment-reasoning leaf (`outbound_comment_reasoning.go`) out of the
composition root. The prior plan targeted `internal/outbound`; **feasibility-before-code
found that target is wrong** and would corrupt an existing boundary. No code written.

## Component / domain
FB comment-intelligence usecase (P2c knowledge-grounded comment decision).

---

# FEASIBILITY RESULT (2026-06-28) — STOP: `internal/outbound` is the wrong home

## 1. `internal/outbound` is the vertical-NEUTRAL spine — it forbids this file
`internal/outbound/doc.go` states the package is the *vertical-neutral outbound
coordination spine* (queue, dedup, claim/CAS/lease, transition, finalize, policy,
ledger). Its documented rule:

> **Forbidden imports:** any service (`services/facebook`, …), drivers/copilot, fburl,
> jobhandlers, internal/server. Outbound must stay vertical-neutral. It OWNS the
> ActionExecutor port — services implement it; **outbound never imports a service**.

`outbound_comment_reasoning.go` imports `internal/services/facebook` (calls
`ResolveCommentIdentity`), `internal/ai` (`DecideComment`/`GenerateCommentV2`/
`EvaluateGate`/`ApplyGate`/`LoadOrgCommentPolicies`), and
`workspace_knowledge/runtime` (`CandidatesForLead`). Moving it INTO `internal/outbound`
would make the neutral spine import a vertical service + AI content generation —
a direct violation of the package's stated architecture role.

## 2. The DI seam does not rescue it
ARCHCM2b previously planned to inject `facebook.ContactDirectory` to drop the concrete
`fbContactDirectory` reference. That removes ONE coupling, but the file still calls
`facebook.ResolveCommentIdentity(...)` and uses `ai.*` throughout. To make it truly
neutral, ALL of comment generation / gate evaluation / identity resolution / knowledge
candidates would have to be inverted behind outbound-owned ports — a broad new
abstraction over FB+AI content logic. That is an explicit STOP condition
(CLAUDE.md: "a safe move needs a broad port/abstraction").

## 3. This is not unique to comment_reasoning — all of L3 is FB-coupled
Import scan of the L3 "core": **all three** files import `internal/services/facebook`;
comment_reasoning + lead_pipeline also import `internal/ai` + `workspace_knowledge/runtime`.
So the ARCHCM2 umbrella's premise — "move the L3 outbound core into `internal/outbound`"
— is wrong for the whole cluster: this code is **Facebook + AI content/resolution**,
not vertical-neutral coordination. The neutral spine is the wrong destination for ALL of it.

`comment_reasoning` is an **application/usecase orchestrator** (composes a service +
ai + knowledge), which the layer map already allows to live at the composition root
or in a usecase package — NOT in the neutral outbound domain.

## 4. Options (correct home for the FB comment-intelligence usecase)
- **Option A (recommended): keep it in cmd for now; do NOT move into `internal/outbound`.**
  Mark the ARCHCM2 umbrella's L3 target as mis-specified. comment_reasoning is a
  cross-service usecase orchestrator; the composition root is a legitimate home until a
  dedicated usecase package is justified. Zero risk, unblocks nothing falsely, and
  prevents an architecture-corrupting move. The DI seam (inject `facebook.ContactDirectory`)
  can still be done later as a small GREEN readability prep if desired, independent of any move.
- **Option B: move to a FB comment-intelligence usecase package** (e.g.
  `internal/services/facebook/comment` or a new `internal/comment`), where importing
  `ai` + `facebook` + `knowledge` is allowed. A real YELLOW move, but needs the
  package-home decision (where the FB comment usecase lives) + MODULE_BOUNDARIES sign-off.
- **Option C: invert FB+AI behind outbound-owned ports and move to `internal/outbound`.**
  REJECTED: broad abstraction over content generation; not a leaf-move; violates the
  "no broad port" stop; high risk for no boundary payoff.

## 5. Recommended default: **Option A**
Do not move it into the neutral spine. Keep comment_reasoning in cmd; correct the
ARCHCM2 umbrella so L3's destination is the FB usecase side, not `internal/outbound`.
Re-open ARCHCM2b only once Option B's usecase-home is decided.

## 6. Impact on the ARCHCM2 umbrella
The umbrella (and ARCHCM2c) assume `internal/outbound` as the L3 destination. That is
incorrect for FB-coupled content logic. ARCHCM2 should be re-framed: only the
**vertical-neutral** parts of the cmd outbound surface (queueing/dedup/policy call
orchestration) could approach `internal/outbound`; the FB+AI content (comment_reasoning,
lead_pipeline content, lead_outcome formatting) belongs on the FB usecase side. This
needs a corrected umbrella target before ARCHCM2c is planned.

## Validation
N/A (feasibility re-scope — no production code).

## Done criteria
Re-scoped: target corrected away from the neutral `internal/outbound`; comment-usecase
home decision recorded (Option A: stay in cmd; Option B: FB usecase package). Stays
BLOCKED until the home decision is taken.
