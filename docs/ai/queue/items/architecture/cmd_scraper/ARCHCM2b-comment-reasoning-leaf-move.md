---
id: ARCHCM2b
status: REVIEW
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM1]
parallel_safe: false
branch: "chore/archcm2b-commenting-usecase-move"
pr_url: ""
boundary_target: leaf-move
target_package: internal/services/facebook/commenting
audit_status: COMPLETE
---

# ARCHCM2b — Move comment_reasoning to a Facebook comment usecase package

## IMPLEMENTED (2026-06-28, branch chore/archcm2b-commenting-usecase-move)
Feasibility §8.1 confirmed (no import cycle: `services/facebook` already imports `ai`;
neither `ai` nor `workspace_knowledge/runtime` imports `services/facebook`; the new
package is imported by nothing). Done:
- New `internal/services/facebook/commenting/comment_reasoning.go` (113) — `Mode()`,
  `Apply()`, `Input` (verbatim logic, exported names).
- DI seam: `Apply` takes an injected `facebook.ContactDirectory` (Input.Contacts)
  instead of building the concrete `fbContactDirectory`; the cmd caller passes
  `fbContactDirectory{c.db}`. `fbContactDirectory` stays in cmd.
- Deleted `cmd/scraper/outbound_comment_reasoning.go`; caller
  `outbound_lead_pipeline.go` switches to `commenting.*` (kept at 200 lines).
- `internal/outbound` untouched; no queue/RBAC/CAS/ledger/runtime semantics changed.
- New characterization test `TestMode` (off/dryrun/live + alias + case/space).
Validation: go build/test ./... green; topology + cognitive + file-size guards pass.

## Goal
Move the comment-reasoning leaf (`outbound_comment_reasoning.go`) out of the
composition root into a **Facebook-specific comment usecase package**
(`internal/services/facebook/commenting`). The prior plan targeted `internal/outbound`;
**feasibility-before-code found that target is wrong** (it is the vertical-neutral
spine). Target corrected; direction decided. No code written in this audit.

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
- **Option A — keep it in `cmd/scraper`. TEMPORARY BASELINE ONLY, not the final
  architecture.** A safe no-op: the file compiles and works where it is. But
  `cmd/scraper` is the composition root / facade, NOT the long-term home for Facebook +
  AI comment-intelligence usecase logic. Acceptable only until Option B lands; must not
  be treated as the destination.
- **Option B (RECOMMENDED) — move to a Facebook-specific comment usecase package:**
  `internal/services/facebook/commenting` (repo-conventional: `internal/services/facebook`
  already hosts `comment_quality.go` / `comment_readiness.go`; a `commenting/` subpackage
  is the bounded SRP home and avoids growing the flat `comment_*` prefix). There,
  importing `ai` + `facebook` + `knowledge` is legitimate (FB usecase orchestration).
  `cmd/scraper` keeps a thin adapter and calls the usecase. YELLOW move via a narrow DI
  seam (inject `facebook.ContactDirectory` from cmd).
- **Option C — invert FB+AI behind outbound-owned ports and move to `internal/outbound`.**
  REJECTED: a broad abstraction over content generation just to satisfy a wrong target;
  not a leaf-move; violates the "no broad port" stop; high risk for no boundary payoff.

## 5. Recommended default: **Option B** — Facebook comment usecase package

## 6. Founder / architect direction (recorded 2026-06-28)
- `internal/outbound` **stays vertical-neutral** — no FB/AI content logic moves into it.
- Facebook + AI comment-intelligence **belongs on the Facebook service/usecase side**
  (`internal/services/facebook/commenting` or the closest repo-conventional equivalent).
- `cmd/scraper` **builds adapters and calls the usecase** — it is the composition
  root/facade, not the long-term home for this logic.
- **No broad abstraction** invented just to satisfy a wrong target (Option C stays
  rejected).

## 7. Impact on the ARCHCM2 umbrella
The umbrella (and ARCHCM2c) previously assumed `internal/outbound` as the L3
destination — wrong for FB-coupled content. Corrected target: the FB usecase side
(`internal/services/facebook/...`). Only genuinely **vertical-neutral** parts of the cmd
outbound surface (queueing/dedup/policy call orchestration) could approach
`internal/outbound`; the FB+AI content (comment_reasoning, lead_pipeline content,
lead_outcome formatting) goes to the FB usecase side. The umbrella target is updated
accordingly (see its banner).

## 8. Next implementation slice (ARCHCM2b is READY)
1. **Feasibility-check first** (Boundary Playbook §3): confirm
   `internal/services/facebook/commenting` importing `ai` + `workspace_knowledge/runtime`
   creates no import cycle (i.e. neither imports `internal/services/facebook`); if a
   cycle exists, stop and re-scope the seam.
2. Create `internal/services/facebook/commenting` with a package doc.
3. Move `outbound_comment_reasoning.go` there (`CommentReasoningMode`,
   `ApplyCommentReasoning`, `CommentReasoningInput`); the `facebook` import becomes
   in-package.
4. **Narrow DI seam:** inject `facebook.ContactDirectory` (the existing interface) into
   `ApplyCommentReasoning` instead of constructing the concrete `fbContactDirectory`;
   `cmd/scraper` (`outbound_lead_pipeline.go`) builds the adapter and passes it.
5. Caller switches to `commenting.*`. Keep behavior unchanged; add a characterization
   test for the off/dryrun/live mode decision.
6. **Do not** touch queue / RBAC / CAS / ledger / runtime semantics; keep
   `internal/outbound` untouched.

## Validation
N/A for this audit (no production code). The implementation PR runs:
go build/test ./... ; scripts/check_topology.sh ; scripts/go_cognitive_check.sh ;
scripts/check_file_size.py ; ai_validate.sh. New Code Sonar clean.

## Done criteria
This audit: target corrected to the FB comment usecase package; Option B recorded as
the direction; ARCHCM2b READY. Implementation (separate PR): `comment_reasoning` lives
in `internal/services/facebook/commenting` behind a facade with an injected
`facebook.ContactDirectory`; caller updated; no import cycle; behavior + tests green;
`internal/outbound` untouched; no queue/RBAC semantics change.
