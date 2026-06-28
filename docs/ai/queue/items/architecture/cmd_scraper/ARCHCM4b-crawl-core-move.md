---
id: ARCHCM4b
status: REVIEW
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM4a]
parallel_safe: false
branch: "chore/archcm4b-crawl-core-move"
pr_url: ""
boundary_target: transport-to-usecase
---

# ARCHCM4b — Move the typed crawl core into internal/crawler

## IMPLEMENTED (2026-06-29, branch chore/archcm4b-crawl-core-move)
Feasibility confirmed: no import cycle (move set is arg-free, imports only
jobs/store/connectors/browsergateway/models/store-crawl/textutil; crawler imported by
nothing); small facade (2 exports). The only real external coupling was a cmd test on
`connectorCrawlEnvelopeForTask` — moved with its function. `check_topology` clean.
- **Created** `internal/crawler/{request,dispatch,recurring}.go` + `dispatch_test.go`:
  `SubmitCrawlRequest` + `CrawlRequest` (exported), and package-private
  `submitConnectorCrawl` / `connectorCrawlEnvelope(+ForTask)` /
  `enqueueConnectorCrawlCommand` / `pickOnlineConnectorForCrawl` /
  `rememberRecurringCrawlIntents` / `isRecurringCrawlSource` — all verbatim.
- **cmd**: `submitOpenCrawl` now calls `crawler.SubmitCrawlRequest`; `resolveCrawlRequest`
  returns `crawler.CrawlRequest`. `resolveCrawlRequest` + `crawl_account_pick.go` (RBAC)
  + the scheduler stay in cmd. crawl_runtime.go shrank 307 → 105 (off the allowlist).
- Verbatim move → dispatch ladder / 5-min freshness / server fallback / deterministic
  task ids / claim scheduler / retry+envelope refusal / connector-command semantics all
  unchanged. RBAC (auto-pick owner filter + explicit-account pass-through) untouched in
  cmd. No queue/CAS/ledger/schema/auth change. ARCHCM-R2a / ARCHCM-R2b not touched.
Validation: go build/vet/test ./... green; topology + cognitive + file-size guards pass.

## Goal
After ARCHCM4a's de-arg seam, move the typed plan-assembly + recurring scheduler +
connector-dispatch core from cmd/scraper into `internal/crawler` behind a cmd facade.
Behavior-preserving (founder-approved ARCHCM-R2 Option A).

## Component / domain
crawler runtime + scheduling. RED-adjacent (connector dispatch + command creation).

## What moves (verbatim) + what stays
- **Moves to `internal/crawler`:** typed `submitOpenCrawl` core, task assembly,
  `submitConnectorCrawl` + `pickOnlineConnectorForCrawl` + `enqueueConnectorCrawlCommand`
  + `connectorCrawlEnvelope*` (verbatim dispatch), the recurring scheduler
  (`runCrawlIntentScheduler`/`scheduleDueCrawlIntents`/`rememberRecurringCrawlIntents`),
  and the pure helpers (`openCrawlTaskID`/`recurringCrawlTaskID`/`isRecurringCrawlSource`/
  RFC3339 helpers).
- **Stays in cmd:** the arg/prompt resolution facade (ARCHCM4a) + `fbContactDirectory`-style
  adapters; the ~5 callers (`agent_actions.go`, `direct_post_intake.go`) and
  `main.go` scheduler wiring switch to `crawler.*`.

## Dependencies
ARCHCM4a (de-arg seam established first).

## Risk notes
YELLOW move crossing an import boundary into a RED-adjacent runtime. The connector
dispatch + command creation move **verbatim** — preserve every ARCHCM4 §10 invariant
byte-for-byte (esp. #6 auto-pick owner filter, #7 explicit-account pass-through, #9
connector-command semantics, #10 no CAS/lease/ledger/schema/auth touch). Verify no
import cycle (`internal/crawler` → jobs/store/connectors/models/browsergateway; none
import crawler). check_topology + WORKER_NO_TRANSPORT warn-only must stay clean.
Characterization tests for the dispatch ladder + scheduler before the move.

## Validation
go build/vet/test ./... ; scripts/check_topology.sh ; scripts/go_cognitive_check.sh ;
scripts/check_file_size.py ; ai_validate.sh ; git diff --check.

## Done criteria
crawl runtime/scheduler/dispatch in `internal/crawler`; cmd only wires + parses args;
~5 callers + main.go on `crawler.*`; no import cycle; all ARCHCM4 §10 invariants hold;
guards green. On merge, ARCHCM4 umbrella → DONE; ARCHCM3 still gated on its own deps.
