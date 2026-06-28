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
role: umbrella
decomposed_into: [ARCHCM2a, ARCHCM2b, ARCHCM2c, ARCHCM2d]
note: "umbrella BLOCKED on ARCHCM2a (L2 home); ARCHCM2b (comment_reasoning leaf) is independently READY"
---

# ARCHCM2 — Move outbound pipeline out of cmd/scraper into internal/outbound

## Goal
Relocate the leaked outbound business domain (outbound_action_queueing/lead_pipeline/lead_outcome/action_context/comment_reasoning, ~724 LOC) from the composition root into the owning internal/outbound package behind a facade.

## Component / domain
outbound domain. Moves logic OUT of cmd into internal/outbound.

## Dependencies
ARCHCM1 (DONE — pure-helper split); ARCHCM-R1 (DONE — account-scope decision).

---

> **CORRECTION (2026-06-28, ARCHCM2b feasibility):** the destination assumed below —
> `internal/outbound` — is the **vertical-NEUTRAL** coordination spine and forbids
> importing `services/facebook` + `ai` (see `internal/outbound/doc.go`). The entire L3
> "core" (comment_reasoning, lead_pipeline, lead_outcome) imports `services/facebook`
> (and mostly `ai` + knowledge), so it is FB+AI **content** logic and does NOT belong in
> the neutral spine. **Corrected target (founder-directed):** the FB usecase side —
> `internal/services/facebook/...` (e.g. `internal/services/facebook/commenting`), NOT
> `internal/outbound`. `cmd/scraper` builds adapters and calls the usecase. See ARCHCM2b
> Option B. ARCHCM2c inherits this corrected destination.

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

## 3. L3 movability — comment_reasoning is near-clean (one DI seam), the rest are coupled
Full-file read of the L3 files (correcting an earlier bareword *function-call* scan
that missed struct-literal type references):

- `outbound_comment_reasoning.go` — **independently movable after one small DI seam**,
  NOT gated by the L2 decision. Its only cmd-local coupling is the concrete adapter
  type `fbContactDirectory{in.db}` at line 94 (a struct literal — invisible to a
  `foo(`-style scan, which is why the first pass wrongly called it "self-contained").
  `fbContactDirectory` (`facebook_contact_directory.go`, ~30-line composition-root
  adapter) already implements the importable `facebook.ContactDirectory` interface, so
  the fix is to inject that interface into `applyCommentReasoning` instead of building
  the concrete adapter inside it. The cmd caller (`outbound_lead_pipeline.go:192`)
  constructs `fbContactDirectory{db}` and passes it. No L2 / queue / RBAC dependency;
  behavior-preserving. → **ARCHCM2b, READY** (depends only on ARCHCM1=DONE).
- `outbound_lead_outcome.go` — reaches cmd-local `formatCommentResult`,
  `formatOutreachResult`, `noEligibleCommentMessage`, `queueOutreachMessage`,
  `recordSkip` (formatting / skip / queue-message helpers defined elsewhere in cmd).
- `outbound_lead_pipeline.go` — the orchestration spine: reaches cmd-local
  `coverageGate`, `businessContextForOrg`, `queueOutreachMessage`, `recordSkip`,
  `formatOutreachResult`, `prepareOutreachContent`, `processOutreachLead`,
  `fbContactDirectory`, and one `argString(args,"template")` (the lone stray `arg*`
  outside L1).

So lead_outcome/lead_pipeline need a real de-coupling prep (lift the shared cmd-local
helpers into the cluster or inject them) before they can move — but
comment_reasoning does not, and moves first as the independent leaf.

## 4. Coupling counts
- `arg*` in the cluster: 19 total, **18 in L1** (queueing.go), 1 stray in lead_pipeline.
- External callers needing a facade switch: `queueLeadOutreach` (3 files),
  `queueGroupPost` (1), `queueProfilePost` (1), `resolveCallerAccountID` (1 + crawl).
- Cluster → account-scope calls: **0** (no cycle from the cluster side).
- account-scope → cluster: `facebook_account_scope.go` → `queueLeadOutreach` (1, one-way).
- Queue writes are **store methods** (`QueueOutboundForOrg`, `RecordOutcome`) the
  cluster *calls*; a verbatim move does not alter queue semantics (preserve call sites).

## 5. Options (staging)

- **Option A — Stage it; the L2 decision blocks only L2-dependent work, NOT the
  independent leaf (RECOMMENDED).** Decomposed into four child items:
  1. **ARCHCM2a (decision, RED-adjacent) — BLOCKED:** choose L2's home — a neutral
     `internal/execcontext` (or `internal/identity`) leaf, models-only, imported by
     both outbound and crawl. Founder/architect call (account-scope/RBAC-adjacent).
     Blocks L2 movement and anything that depends on L2 (ARCHCM2c, ARCHCM2d) — but
     **not** ARCHCM2b.
  2. **ARCHCM2b (YELLOW move + DI seam) — READY:** inject `facebook.ContactDirectory`
     into `applyCommentReasoning` (drop the concrete `fbContactDirectory` reference),
     then move `outbound_comment_reasoning.go` → a new `internal/outbound` package with
     a doc + facade. Independent of L2 — `depends_on: [ARCHCM1]` (DONE), so executable
     once this audit merges.
  3. **ARCHCM2c (YELLOW prep+move) — BLOCKED:** lift the cmd-local helpers L3 shares
     (`coverageGate`/`businessContextForOrg`/`queueOutreachMessage`/`recordSkip`/
     `format*`/`fbContactDirectory`) into the cluster or inject them, then move
     lead_outcome + lead_pipeline. `depends_on: [ARCHCM2a, ARCHCM2b]`.
  4. **ARCHCM2d — BLOCKED:** L1 (`queueing.go`) becomes the thin cmd facade over
     `internal/outbound`; external callers switch to `outbound.*`.
     `depends_on: [ARCHCM2b, ARCHCM2c]`.
- **Option B — Big-bang move of all five files into internal/outbound.** REJECTED:
  drags shared L2 into outbound (crawl→outbound wrong-direction), mixes queue-path +
  account-scope + ~10 cmd-helper couplings in one un-reviewable PR.
- **Option C — Defer.** Leave the cluster in cmd. Zero risk, no progress; the
  composition root keeps ~724 LOC of leaked domain.

## 6. Recommended default: **Option A**
Stage the L2-dependent work behind the L2 home decision, but carve out the
independent `comment_reasoning` leaf so it ships now. This keeps dependency direction
correct (crawl and outbound both depend on a neutral exec-context leaf, not on each
other), keeps each move reviewable and behavior-preserving, and preserves forward
progress instead of freezing the whole track.

## 7. First executable slice — ARCHCM2b (READY now, NOT gated on ARCHCM2a)
Inject `facebook.ContactDirectory` into `applyCommentReasoning`, then move
`outbound_comment_reasoning.go` into a new `internal/outbound` package with a package
doc + facade; `cmd` callers (`outbound_lead_pipeline.go`) switch to `outbound.*` and
construct the adapter at the call site. Behavior-preserving; characterization first;
import-cycle / `check_topology.sh` + tenant guards green; New Code Sonar clean.

## Risk notes
The full umbrella move is YELLOW (crosses an import boundary; touches outbound queue
*call sites* — preserve exactly) and RED-adjacent via L2 (account-scope/RBAC home),
so the umbrella stays BLOCKED on ARCHCM2a. The `comment_reasoning` leaf (ARCHCM2b)
carries none of that and is independently executable.

## Validation
N/A (this PR is the feasibility re-scope — no production code). Each staged slice:
go build/test ./... ; ai_validate.sh ; scripts/check_topology.sh.

## Done criteria (umbrella)
Superseded by the staged children: L2 home decided (ARCHCM2a); comment_reasoning
moved (ARCHCM2b); lead_outcome/lead_pipeline de-coupled + moved (ARCHCM2c);
queueing.go is the cmd facade (ARCHCM2d); topology/tenant guards green; no
queue-semantics change. The umbrella stays BLOCKED until all four children are DONE;
ARCHCM2b is independently READY in the meantime.
