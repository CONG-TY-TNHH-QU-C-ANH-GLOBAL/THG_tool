---
id: ARCHCM2
status: BLOCKED
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM1, ARCHCM-R1]
parallel_safe: false
branch: "audit/archcm2-feasibility-restage"
pr_url: ""
blocked_on: execution-context-home-decision
boundary_target: transport-to-usecase
audit_status: COMPLETE
---

# ARCHCM2 — Move outbound pipeline out of cmd/scraper into internal/outbound

## Goal
Relocate the leaked outbound business domain (outbound_action_queueing/lead_pipeline/lead_outcome/action_context/comment_reasoning, ~724 LOC) from the composition root into the owning internal/outbound package behind a facade.

## Component / domain
outbound domain. Moves logic OUT of cmd into internal/outbound.

## Dependencies
ARCHCM1 (DONE — pure-helper split); ARCHCM-R1 (DONE — account-scope decision).

---

# FEASIBILITY & RE-SCOPE (verified 2026-06-28 — move-only big-bang is NOT possible; reclassified to staged + BLOCKED on one decision)

The "move ~724 LOC into internal/outbound behind a facade" framing does not hold.
The `outbound_*` cluster is not one domain — it is **three layers with different
owners**, and one of them (L2) is shared with crawl and account-scope, so a single
move would point dependencies the wrong way. Verified by import + call-graph scan.

## 1. The cluster is three layers, not one

| Layer | Files | Nature | Correct home |
|---|---|---|---|
| **L1 — arg facade** | `outbound_action_queueing.go` (165) | composition-root glue: the `queue*` entry points that read `args map[string]any` (**18 of 19 `arg*` calls in the whole cluster live here**) and call L2 resolution + L3 core | **stays in cmd** (thin adapter over `internal/outbound`) |
| **L2 — execution-context resolution** | `outbound_action_context.go` (118): `resolveCallerAccountID`, `resolveUserActionContext`, `ownedAccountCandidates`, `callerAccountForExplicitID`, `selectExecutionAccount` | account/exec-context resolution, **shared by outbound AND crawl** (`crawl_runtime.go` + `execution_context_test.go` call it) and entangled with account-scope (`callerRestrictedToOwnedAccounts`) | **NOT internal/outbound** — a neutral shared leaf (see decision) |
| **L3 — outbound domain core** | `outbound_comment_reasoning.go` (105), `outbound_lead_outcome.go` (132), `outbound_lead_pipeline.go` (200) | the genuine outbound business logic | **internal/outbound** |

## 2. Why a single move into internal/outbound is wrong
- **L2 is shared.** `crawl_runtime.go` calls `resolveUserActionContext` /
  `resolveCallerAccountID`. If L2 moves into `internal/outbound`, then
  `cmd/scraper` crawl code must import `internal/outbound` for account resolution —
  a crawl→outbound dependency that is the wrong direction and couples two verticals.
  L2 belongs in a neutral shared leaf, not the outbound package.
- **L2 is account-scope/RBAC-adjacent.** It depends on `callerRestrictedToOwnedAccounts`
  (the ARCHCM-R1 OWNER classification). Choosing its home is a boundary decision, not
  a mechanical move — RED-adjacent.

## 3. Even L3 is not a clean leaf yet (de-coupling prep needed)
Call-graph scan of the L3 files for cmd-local (non-cluster, non-import) symbols:
- `outbound_comment_reasoning.go` — **self-contained** (only cluster-internal
  `commentReasoningMode`/`applyCommentReasoning` + imports). The one clean leaf.
- `outbound_lead_outcome.go` — reaches cmd-local `formatCommentResult`,
  `formatOutreachResult`, `noEligibleCommentMessage`, `queueOutreachMessage`,
  `recordSkip` (formatting / skip / queue-message helpers defined elsewhere in cmd).
- `outbound_lead_pipeline.go` — the orchestration spine: reaches cmd-local
  `coverageGate`, `businessContextForOrg`, `queueOutreachMessage`, `recordSkip`,
  `formatOutreachResult`, `prepareOutreachContent`, `processOutreachLead`, and one
  `argString(args,"template")` (the lone stray `arg*` outside L1).

So L3 needs a small de-coupling prep (lift the shared cmd-local helpers into the
cluster or inject them) before lead_outcome/lead_pipeline can move cleanly.
comment_reasoning can move first.

## 4. Coupling counts
- `arg*` in the cluster: 19 total, **18 in L1** (queueing.go), 1 stray in lead_pipeline.
- External callers needing a facade switch: `queueLeadOutreach` (3 files),
  `queueGroupPost` (1), `queueProfilePost` (1), `resolveCallerAccountID` (1 + crawl).
- Cluster → account-scope calls: **0** (no cycle from the cluster side).
- account-scope → cluster: `facebook_account_scope.go` → `queueLeadOutreach` (1, one-way).
- Queue writes are **store methods** (`QueueOutboundForOrg`, `RecordOutcome`) the
  cluster *calls*; a verbatim move does not alter queue semantics (preserve call sites).

## 5. Options (staging)

- **Option A — Stage it: decide L2 home → move comment_reasoning → de-couple+move
  the rest (RECOMMENDED).**
  1. **ARCHCM2a (decision, RED-adjacent):** choose L2's home — a neutral
     `internal/execcontext` (or `internal/identity`) leaf, models-only, imported by
     both outbound and crawl. Founder/architect call (account-scope/RBAC-adjacent).
  2. **ARCHCM2b (YELLOW move):** move `outbound_comment_reasoning.go` → `internal/outbound`
     (the one self-contained leaf), establish the package + facade, migrate its test.
  3. **ARCHCM2c (YELLOW prep+move):** lift the cmd-local helpers L3 shares
     (`coverageGate`/`businessContextForOrg`/`queueOutreachMessage`/`recordSkip`/
     `format*`) into the cluster or inject them, then move lead_outcome + lead_pipeline.
  4. **ARCHCM2d:** L1 (`queueing.go`) becomes the thin cmd facade over `internal/outbound`;
     external callers switch to `outbound.*`.
- **Option B — Big-bang move of all five files into internal/outbound.** REJECTED:
  drags shared L2 into outbound (crawl→outbound wrong-direction), mixes queue-path +
  account-scope + ~10 cmd-helper couplings in one un-reviewable PR.
- **Option C — Defer.** Leave the cluster in cmd. Zero risk, no progress; the
  composition root keeps ~724 LOC of leaked domain.

## 6. Recommended default: **Option A**
Stage behind the L2 home decision. It is the only path that keeps dependency
direction correct (crawl and outbound both depend on a neutral exec-context leaf,
not on each other) and keeps each move reviewable and behavior-preserving.

## 7. Exact next slice (once L2 home is decided in ARCHCM2a)
**ARCHCM2b** — move `outbound_comment_reasoning.go` into a new `internal/outbound`
package with a package doc + facade; migrate `outbound_neutral_contract_test.go`
coverage as needed; `cmd` callers (`applyCommentReasoning`) switch to `outbound.*`.
Behavior-preserving; characterization first; `check_topology.sh` + tenant guards green.

## Risk notes
YELLOW move that crosses an import boundary; touches the outbound queue *call sites*
(preserve exactly). RED-adjacent via L2 (account-scope/RBAC home). Gate the clean
move behind ARCHCM2a.

## Validation
N/A (this PR is the feasibility re-scope — no production code). Each staged slice:
go build/test ./... ; ai_validate.sh ; scripts/check_topology.sh.

## Done criteria
Superseded by the staged plan: L2 home decided (ARCHCM2a); comment_reasoning moved
(ARCHCM2b); lead_outcome/lead_pipeline de-coupled + moved (ARCHCM2c); queueing.go is
the cmd facade (ARCHCM2d); topology/tenant guards green; no queue-semantics change.
Stays BLOCKED until ARCHCM2a (the L2 execution-context home) is decided.
