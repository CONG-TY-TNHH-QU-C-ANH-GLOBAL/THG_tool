---
id: ARCHCM2a
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
blocked_on: founder-architect-decision
boundary_target: blocked-decision
---

# ARCHCM2a — DECISION: home for the L2 execution-context resolution layer

## Goal (decision-only — account-scope / RBAC-adjacent)
Decide where the L2 execution-context resolution layer lives. Today it sits in
`cmd/scraper/outbound_action_context.go` (`resolveCallerAccountID`,
`resolveUserActionContext`, `ownedAccountCandidates`, `callerAccountForExplicitID`,
`selectExecutionAccount`) but is **shared by outbound AND crawl** and is entangled
with the ARCHCM-R1 OWNER classification (`callerRestrictedToOwnedAccounts`). See the
ARCHCM2 feasibility re-scope for the full layer map.

## Why this is a decision, not a move
- `crawl_runtime.go` calls `resolveUserActionContext` / `resolveCallerAccountID`, so
  L2 cannot move into `internal/outbound` without creating a crawl→outbound
  dependency (wrong direction; couples two verticals).
- L2 is account-scope / RBAC-adjacent (it gates which account a caller may act on),
  so its home is a boundary decision requiring human sign-off — not autopilot.

## Options
- **Option A (recommended): neutral `internal/execcontext` leaf.** models-only deps,
  imported by both `internal/outbound` (future) and the cmd crawl path. Keeps
  dependency direction correct; both verticals depend on a shared leaf, not on each
  other. `callerRestrictedToOwnedAccounts` moves with it (it is models-only).
- **Option B: `internal/identity` leaf.** Same shape; co-locate with other identity /
  ownership helpers if one emerges. Pick A vs B on where account-scope helpers should
  consolidate long-term.
- **Option C: leave L2 in cmd.** Then outbound's L3 move proceeds but L2 stays in the
  composition root; revisit later. Lowest churn, least architectural payoff.

## Blocks
L2 movement and every item that depends on L2: **ARCHCM2c, ARCHCM2d**. Does NOT block
**ARCHCM2b** (the comment_reasoning leaf carries no L2 dependency).

## Validation
N/A (decision). The eventual L2 move PR needs RBAC/account-scope characterization
tests + `check_topology.sh` (import direction) + tenant guards.

## Done criteria
A decision record naming L2's home package + import-direction rule, approved by a
human. Until then, status stays BLOCKED.
