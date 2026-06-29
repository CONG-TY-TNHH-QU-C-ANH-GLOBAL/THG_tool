---
id: ARCHCP3c
status: DONE
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCP3a, ARCHCP3b]
parallel_safe: false
branch: "chore/archcp3c-intent-textnorm-decouple"
pr_url: https://github.com/CONG-TY-TNHH-QU-C-ANH-GLOBAL/THG_tool/pull/170
boundary_target: prep-extraction
---

# ARCHCP3c — Decouple the intent cluster's fold/containsAny from the copilot shims

## Why
A cycle prerequisite for ARCHCP3 phase 3 (the `copilot/intent` move): the intent files
(`intent_router.go`, `intent_entities.go`) call `foldVietnameseForMatch` /
`containsAnyFolded`, which are package-`copilot` shims (ARCHCP3a). If the intent files
moved to `intent/` while still calling those shims, `intent → copilot` would cycle.
Safe to do now (not in the abandoned phase-2 bulk rewrite) because ARCHCP3b dropped the
classifier to ≤15, so rewriting calls inside it no longer trips the cognitive guard.

## What this slice does
Point the intent cluster's 18 fold/containsAny calls (intent_router 13, intent_entities
5) directly at `textnorm.Fold` / `textnorm.ContainsAny`; add the `textnorm` import to
both. No agent_*/brain_*/routing_* caller churn (their 24 shim calls are untouched —
those callers do not move, so the shims stay in copilot for them). intent_router 189→193,
intent_entities 146 (both <200); all functions ≤15; no RED zone.

## Behavior preservation
`textnorm.Fold`/`ContainsAny` are the exact functions the shims already delegate to, so
this is a no-op at runtime. Pinned by `intent_router_test.go` (all 7 routes + no-match +
multi-URL + ask-for-link + RouteDecisionFor flags) — passes unchanged.

## Rollback
Revert: the intent files call the copilot shims again. Pure call-target change.

## Remaining for ARCHCP3 phase 3 (the move)
- `stripDashboardContext` is broadly shared (9 copilot files incl. agent_*/routing_*),
  so it cannot move into `intent/`; the intent classifier depends on it → still a
  cycle blocker. It needs its own neutral home (a copilot prompt-prep leaf) before the
  move — the next prep slice (ARCHCP3d).
- The intent-symbol external-ref rewrite touches near-/over-200 caller files
  (brain_action_prep 197, routing_decision 236, agent.go 497) — file-size-aware.

## Validation
go build/vet/test ./... green; check_topology + go_cognitive_check + check_file_size +
import-boundary (no new violation) + ai_validate pass. On merge → DONE.
