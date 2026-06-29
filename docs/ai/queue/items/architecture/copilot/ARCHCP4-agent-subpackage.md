---
id: ARCHCP4
status: BLOCKED
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCP3]
parallel_safe: false
branch: "audit/archcp4-leave-flat-decision"
pr_url: ""
blocked_on: founder-decision-agent-split-vs-leave-flat
boundary_target: blocked-decision
audit_status: COMPLETE
---

# ARCHCP4 — (Optional) Extract copilot/agent subpackage

# DECISION (2026-06-29, senior-architect feasibility) — LEAVE FLAT for now; brain extraction is the only safe code slice and is optional

Post-ARCHCP3 the flat `internal/drivers/copilot` still trips the **warn-only**
component-structure guard: 20 source files (>15) and 16 `agent_*` files (>5, the guard
counts `_test.go`). A senior-architect feasibility pass found:

- **The trigger-clearing move is unsafe.** Only re-namespacing the `agent_*` prefix
  silences the `>5` trigger, but those files are methods on `*Agent` in a dense mutual
  call graph with `agent.go`; moving them either drags `Agent` (breaking the external
  `copilot.Agent` / `NewAgent` surface used by cmd/scraper + 4 internal/server files) or
  creates `sub ↔ root.Agent` import cycles. That is the YELLOW/RED seam ARCHCP4 explicitly
  says NOT to move speculatively.
- **The only clean code slice is OPTIONAL and does not clear the guard.** The `brain_*`
  cluster (brain_types/client/plan_validator/action_prep) is a cohesive, cycle-free leaf
  (imports zero copilot-root symbols; only `NewBrainClient` is external) and could move
  to `internal/drivers/copilot/brain/` wrapper-first. But it carries the `brain` prefix,
  so it leaves the `>15 source` trigger at 16 and the `>5 agent_*` trigger unchanged —
  a cohesion win only, not a guard-clear.

**Decision: leave the package flat for now** (ARCHCP4's own done-criteria permits this).
Forcing the `agent_*` split is unsafe; the optional `brain/` extraction is a wide-churn
de-stutter rename for a warn-only, non-unblocking gain — i.e. exactly the speculative
move the item forbids. The component guard is warn-only and does not fail CI.

## Options
- **Option A (recommended): leave flat.** Accept the warn-only trigger. No code change.
  The package is materially smaller after ARCHCP3 (intent/textnorm/promptprep extracted);
  the residual `agent_*` density is inherent to the orchestrator and not worth an unsafe
  cycle-prone split.
- **Option B: optional `brain/` cohesion PR.** Safe, cycle-free, wrapper-first (keep
  `NewBrainClient` + a `BrainClient` alias in root; export the ~11 brain helpers). Real
  SRP win, but does NOT clear the guard and is a wide rename — do only if the cohesion is
  independently wanted. Founder call.
- **Option C: the `agent_*` re-namespace.** RED/YELLOW — needs an `Agent`-method seam
  (DI/port to break the `*Agent` mutual graph) + export-count + cycle work + cmd/server
  importer handling. Audit-only; do NOT auto-execute. Separate scoped item if pursued.

Stays BLOCKED on the founder choosing A (leave flat / close ARCHCP4) vs B (do the optional
brain PR) vs C (scope the agent split).

## Goal
After intent/ is stable, optionally move the agent orchestration cluster into copilot/agent/. Only do this if the flat package still trips the >15-file trigger after ARCHCP1-3.

## Component / domain
internal/drivers/copilot agent orchestration.

## Files likely involved
agent.go + agent_*.go + brain_*.go (post-split) + tests → internal/drivers/copilot/agent/.

## Dependencies
ARCHCP3.

## Risk notes
YELLOW move-only, larger blast radius (server/cmd import the agent entrypoints). Re-evaluate necessity first — agent/ staying flat at <15 files is acceptable. Do not move speculatively.

## Validation
go build ./... ; go test ./... ; ai_validate.sh

## Done criteria
Either a clean move-only extraction with all importers updated, OR a documented decision to leave flat. No behavior change.
