---
id: ARCHCP3d
status: REVIEW
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCP3c]
parallel_safe: false
branch: "chore/archcp3d-promptprep-leaf"
pr_url: ""
boundary_target: leaf-move
---

# ARCHCP3d — Extract stripDashboardContext to a promptprep leaf (last cycle prereq)

## Why
The final cycle prerequisite for ARCHCP3 phase 3 (the `copilot/intent` move): the intent
classifier (intent_router/intent_entities) calls `stripDashboardContext`, which is a
package-`copilot` helper shared by 9 files (agent_*/routing_*/intent_*). If the intent
files moved to `intent/` while calling it, `intent → copilot` would cycle. It is copilot
prompt-prep (knows the "Dashboard context:" marker), so it does NOT belong in the generic
`textnorm` leaf — it gets its own neutral copilot prompt-prep leaf.

## What this slice does (mirrors ARCHCP3a + ARCHCP3c)
- **New `internal/drivers/copilot/promptprep`** (pure `strings`-only neutral leaf):
  `StripDashboardContext` (verbatim).
- **intent_normalize.go:** `stripDashboardContext` is now a thin shim →
  `promptprep.StripDashboardContext`, so the 12 agent_*/routing_* call sites stay
  unchanged. (intent_normalize.go now holds only the three shims; `strings` import dropped.)
- **Migrated** the 8 intent-cluster calls (intent_router 7, intent_entities 1) to
  `promptprep.StripDashboardContext` directly + added the import. intent files now ≤200,
  functions ≤15, no RED zone.
- New `promptprep_test.go` characterizes StripDashboardContext (marker block / trim /
  marker-at-start / empty / substring-not-stripped).

## Effect — intent cluster is now leaf-clean
After 3a (textnorm) + 3c (intent fold→textnorm) + 3d (this), the copilot/intent files
(intent_router, intent_entities, intent_lexicon, intent_types) depend ONLY on neutral
leaves (`textnorm`, `promptprep`, `fburl`) + each other — NO package-`copilot` symbols.
Phase 3 (the move into `internal/drivers/copilot/intent/`) is now cycle-free; the only
remaining concern is file-size on the external-ref caller rewrite (brain_action_prep 197,
routing_decision 236, agent.go 497) — handle file-size-aware.

## Behavior preservation
`promptprep.StripDashboardContext` is the verbatim function; the shim delegates. Pinned by
`promptprep_test.go` + `intent_router_test.go` (all 7 routes) — pass unchanged.

## Rollback
Revert: the intent files call the copilot shim again; leaf deleted. Pure relocation.

## Validation
go build/vet/test ./... green; check_topology + go_cognitive_check + check_file_size +
import-boundary (no new violation) + ai_validate pass. On merge → DONE.
