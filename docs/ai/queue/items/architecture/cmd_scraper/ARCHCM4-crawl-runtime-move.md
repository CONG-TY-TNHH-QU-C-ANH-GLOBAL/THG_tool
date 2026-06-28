---
id: ARCHCM4
status: BLOCKED
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM-R1, ARCHCM-R2]
parallel_safe: false
branch: "audit/archcm4-feasibility-restage"
pr_url: ""
blocked_on: dearg-seam-prep
boundary_target: transport-to-usecase
audit_status: COMPLETE
role: umbrella
decomposed_into: [ARCHCM4a, ARCHCM4b]
---

# ARCHCM4 — Move crawl runtime/plan/scheduler out of cmd/scraper

## Goal
Relocate crawl_runtime.go (373) + crawl_scheduler.go (172) plan-assembly /
account-resolution / connector-dispatch / scheduling out of the composition root into
internal/crawler. Founder approved the SEMANTICS (ARCHCM-R2 Option A); feasibility
below shows the MOVE is not a single safe move-only PR — it is staged.

## Dependencies
ARCHCM-R1 (DONE), ARCHCM-R2 (DONE — Option A approved 2026-06-29).

---

# FEASIBILITY (verified 2026-06-29 — NOT a single move-only PR; staged)

Semantics are approved and two candidate blockers are cleared, but the move still
needs a de-arg seam first and splits across a RED-adjacent runtime, so it cannot be
one coherent behavior-preserving PR. Exact findings:

## Cleared (NOT blockers)
- **Target `internal/crawler` is correct + exists** (scaffold doc.go). Its forbidden
  list is `internal/server*`, `internal/drivers/{http,telegram,connector}`, `drivers/*`.
- **`browsergateway` is allowed:** the cluster uses only the constant
  `browsergateway.StreamFacebookLoggedIn`; `internal/browsergateway` is a neutral
  provider-capability/constants package, NOT on the forbidden list. No cycle.
- **The "do not move runtime logic casually" rule** in `internal/crawler/doc.go` is
  satisfied — this IS the reviewed refactor (founder-approved ARCHCM-R2 Option A).
- **Semantics:** founder-approved (Option A); a verbatim move preserves them.

## Exact blockers (why it is staged, not move-only)
1. **arg/prompt glue (~31 refs) — package-main, cannot follow into internal/crawler.**
   The cluster threads `args map[string]any` via `argInt64` (×10), `argString` (×16),
   `argBool` (×1) [`action_args.go`] + `maxItemsFromPrompt`/`promptKeywordFallback`/
   `splitKeywords` (×4) [`action_config.go`]. `internal/crawler` cannot import
   `package main`, so the entry points must be **de-arged** (typed params; arg/prompt
   resolution stays in cmd). This is a signature refactor, NOT move-only.
2. **RED-adjacent runtime (12 call sites).** `CreateConnectorCommand` (connector
   command), `appStore.CreateTask/StartTask/FailTask` (task lifecycle),
   `GetLatestConnectorScreenshot`/`ListLocalConnectors`/`PickReadyConnector`/
   `GetExtensionPolicy` (connector dispatch). A verbatim move preserves their
   semantics (founder-approved), but this is the sensitive crawl runtime + the
   internal/crawler "reviewed refactor" surface — it warrants its own reviewable
   slice, NOT bundled with a de-arg.
3. **Size:** 545 LOC moving + ~5 external call sites (`agent_actions.go` ×3,
   `direct_post_intake.go` ×1, `main.go` scheduler wiring) + the scheduler↔runtime
   entanglement (`scheduleDueCrawlIntents` builds args and calls `submitOpenCrawl`).
   De-arg + 545-LOC RED-adjacent move + call sites in one PR is not reviewable and
   mixes a signature refactor with a runtime relocation.

## Staged plan (Option A — recommended)
- **ARCHCM4a (READY, YELLOW, behavior-preserving, cmd-internal):** introduce the
  de-arg seam. Resolve `args map[string]any` → a typed crawl request at the cmd
  boundary (`resolveCrawlMaxItems/Keywords`, `buildCrawlExtras`, account resolution
  stay in cmd); the execution entry points (`submitOpenCrawl` core) take typed params.
  No package move, no connector-semantics change, no RED touch. Enables the move.
- **ARCHCM4b (BLOCKED on ARCHCM4a, YELLOW / RED-adjacent):** move the typed
  plan-assembly + scheduler + connector-dispatch core to `internal/crawler` behind the
  cmd facade — **verbatim dispatch**, preserving every semantic in the checklist below;
  cmd keeps the arg facade; the ~5 callers + `main.go` switch to `crawler.*`.

Options B (force the whole move in one PR) and C (defer) are rejected: B is the
un-reviewable RED-adjacent big-bang the founder's "stop if unsafe" guards against; C
wastes the approved sign-off. Stage instead.

## Founder semantics-preservation checklist (must hold byte-for-byte across ARCHCM4a+b)
1. Dispatch ladder ORDER + first-match short-circuit.
2. 5-minute screenshot freshness.
3. not-routed → `jobStore.Submit` server fallback.
4. Deterministic `openCrawlTaskID` / `recurringCrawlTaskID`.
5. Claim scheduler + `account_not_selected` permanent-fail (NO first-ready fallback).
6. Auto-pick OWNER filter — unchanged.
7. Explicit `account_id` PASS-THROUGH — unchanged (ARCHCM-R2a NOT fixed here).
8. `RetryPolicy{3,1000ms}` + envelope "no concrete source URL" refusal.
9. Connector command creation / TTL / GC / idempotency — UNCHANGED (ARCHCM-R2b separate).
10. No queue/outbox/action_ledger/CAS/lease/schema/auth/session touch.

## Validation
N/A (this PR is the feasibility re-scope — no production code). Each staged slice:
go build/vet/test ./... ; scripts/go_cognitive_check.sh ; scripts/check_file_size.py ;
ai_validate.sh ; git diff --check.

## Done criteria (umbrella)
Superseded by the staged children: de-arg seam (ARCHCM4a); typed plan/scheduler/dispatch
core moved to internal/crawler behind a cmd facade (ARCHCM4b); the 10-point checklist
holds; guards green. Umbrella stays BLOCKED until both children are DONE; ARCHCM4a is
independently READY now.
