---
id: ARCHCM2d
status: BLOCKED
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM2b, ARCHCM2c]
parallel_safe: false
branch: ""
pr_url: ""
blocked_on: prior-staged-moves
boundary_target: transport-to-usecase
---

# ARCHCM2d — Reduce outbound_action_queueing.go to a thin cmd facade

## Goal
Once the L3 core lives in `internal/outbound` (ARCHCM2b + ARCHCM2c), reduce L1
(`outbound_action_queueing.go`, the `queue*` entry points holding 18/19 of the
cluster's `arg*` calls) to a thin composition-root facade: parse `args map[string]any`,
resolve execution context (L2, per ARCHCM2a), then delegate to `internal/outbound`.

## Component / domain
outbound action queueing entry points (composition-root adapter layer).

## Scope
- `queueLeadOutreach`, `queueGroupPost`, `queueProfilePost`, `queueFacebookPostTargets`
  stay in cmd as thin arg-parsing adapters over `outbound.*`.
- External callers (`facebook_account_scope.go`, direct-link/direct-post intake,
  crawl) keep calling the cmd entry points (no facade churn for them) OR switch to
  `outbound.*` where cleaner — decide during the slice.

## Dependencies
ARCHCM2b (package established) and ARCHCM2c (lead pipeline/outcome moved).

## Risk notes
YELLOW: the entry points orchestrate the outbound queue path — preserve queue/dedup/
policy semantics EXACTLY. Behavior-preserving; this is the final cleanup that closes
the ARCHCM2 umbrella.

## Validation
go build/test ./... ; scripts/check_topology.sh ; ai_validate.sh.

## Done criteria
`outbound_action_queueing.go` is a thin cmd facade over `internal/outbound`; no
business logic left in the composition root for this cluster; queue semantics
identical; guards green. On merge, the ARCHCM2 umbrella → DONE (then ARCHCM3 unblocks).
