---
id: ARCHCM2a
status: REVIEW
lane: YELLOW
risk: YELLOW
depends_on: []
parallel_safe: false
branch: "chore/archcm2a-phase2-execcontext"
pr_url: ""
boundary_target: transport-to-usecase
---

# ARCHCM2a — DECISION: home for the L2 execution-context resolution layer

## PHASE 1 DONE (2026-06-29, branch chore/archcm2a-owner-predicate-to-models)
Realized the neutral home for the **shared** account-scope primitive. Post-ARCHCM4,
the only thing outbound AND crawl still share is the OWNER-restriction predicate
(`callerRestrictedToOwnedAccounts`); the store-coupled L2 resolution
(`resolveCallerAccountID` etc.) is now **outbound-only** (callers:
`outbound_action_queueing.go` + tests — crawl uses its own `crawlOwnershipGate`).
- Moved the pure predicate into `internal/models/permissions.go` as
  `models.RestrictedToOwnedAccounts` (next to `IsAccountOwnerAllowed` /
  `CanViewAccountDevice` / `AccountControlAllowed` — the neutral permission-predicate
  leaf both verticals already import). Deleted cmd's `account_scope_role.go`; the test
  moved to `permissions_test.go`. Behavior-preserving (verbatim predicate); RBAC
  who-can-do-what unchanged. RED-zone touch via safe migration (move-after-topology-
  proof); rollback = move back to cmd.
- **Effect:** the cross-vertical account-scope coupling is gone (single source in
  `models`). The remaining ARCHCM2a question narrows to **phase 2** below.

## PHASE 2 DONE (2026-06-29, branch chore/archcm2a-phase2-execcontext)
Feasibility re-confirmed: the L2 resolution is **outbound-only** (sole external prod
caller `outbound_action_queueing.go` ×3; crawl uses its own `crawlOwnershipGate`). Per
the decision rule (`internal/outbound/...` when truly outbound-only; a top-level neutral
leaf only if neutral reuse is real — it is not), the home is **`internal/outbound/execcontext`**
(outbound-scoped, bounded subpackage), NOT a top-level neutral leaf.
- Moved verbatim: `ResolveUserActionContext` + `ResolveCallerAccountID` (exported) +
  `callerAccountForExplicitID` / `ownedAccountCandidates` / `selectExecutionAccount`
  (package-private). cmd `outbound_action_queueing.go` calls `execcontext.*`. Both
  characterization test files moved into the package (matrix + deterministic-context).
- Import topology proven: `internal/store` does not import `internal/outbound`; the new
  package imports only `store` + `models`; nothing imported it before → clean DAG, no
  cycle (`check_topology` green).
- RED-zone (account-scope/RBAC) touched as a SAFE MIGRATION (move-after-topology-proof):
  who-can-do-what unchanged, no new authoritative path, reversible.

**ARCHCM2a is fully realized** (phase 1: shared predicate → `models`; phase 2: L2
resolution → `internal/outbound/execcontext`). On merge → DONE, which **unblocks
ARCHCM2c** (its `resolveUserActionContext` dependency is now importable from a non-cmd
package, so the lead_pipeline move will not call back into cmd).

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
