---
id: ARCHSV3
status: BLOCKED
lane: RED
risk: RED
depends_on: [ARCHSV2]
parallel_safe: false
branch: ""
pr_url: ""
blocked_on: human-boundary-decision
boundary_target: transport-to-usecase
---

# ARCHSV3 — Extract internal/server/agent/crawl_ingest subpackage

## Feasibility (VERIFIED 2026-06-29 — move-only is NOT possible; reclassified YELLOW→RED)
Same Handler-coupling / import-cycle blocker as [ARCHSV2](ARCHSV2-agent-finalize-subpackage.md);
the "move-only YELLOW" framing does not hold. The crawl_ingest cluster is a `*Handler`-bound,
CAS-adjacent pipeline, not a set of pure functions:

- Every entry point is a `*Handler` method: `processConnectorCrawlResult`, `resolveCrawlOwnership`,
  `handleFailedCrawl`, `buildConnectorCrawlIngestDeps` (crawl_ingest.go), `processConnectorCrawlItem`
  (crawl_ingest_item.go). They reach back through `h.db`, `h.notifier`, `h.tgEvents`, `h.baseURL`,
  `h.aiClass`, and call sibling Handler methods (`h.resolveDirectPostIntake`, `h.failDirectPostImport`,
  `h.evaluateDirectPostCrawlItem`).
- `crawl_ingest_processor.go` defines `type crawlResultProcessor struct { h *Handler; ... }` — the
  struct's first field is `*Handler`, and its methods call back through `p.h.*`.
- The **direct-post CAS** lives in the processor (the item's own risk note says MOVE ONLY, do not
  alter CAS) — a controlled zone.

**Import-cycle blocker:** moving the cluster to `crawl_ingest/` makes `crawl_ingest` import `agent`
(for the `*Handler` its struct field + methods need) while `agent` imports `crawl_ingest`
(orchestration) → an `agent ↔ crawl_ingest` cycle. Breaking it requires replacing `h *Handler` with
a DI port carrying `db`/`notifier`/`tgEvents`/`aiClass` + the three cross-method calls — a broad
abstraction threaded through the direct-post CAS path. That is the §4.3 founder-gated DI-port
refactor, not an autopilot move-only.

## Unified boundary decision (SV2 + SV3 + SV4 are ONE problem)
SV2 (finalize), SV3 (crawl_ingest), and SV4 (outbox) are the same Handler-coupled, transport/CAS-
adjacent flat `internal/server/agent` package. Each "move-only subpackage" item hits the identical
`agent ↔ sub` cycle. They should be decided together, not branched on one at a time:
- **Option A — defer all (recommended):** leave the agent package flat. The prefix smell
  (13×crawl_/5×finalize_/4×outbox_) is cosmetic; the methods are already package-private; ARCHSV1
  already trimmed the package. Zero churn, zero risk.
- **Option B — one deliberate DI-port refactor (founder-approved, behavior-risk, staged):** decouple
  the agent Handler into injected dependency ports, then move all three clusters. Needs idempotency/
  CAS-replay tests guarding the direct-post + execution_id gates; staged additive-port → move PRs.
  Out of scope for an autopilot move.
- **Option C — GREEN pure-free-function sub-slices only:** extract just the cluster's pure helpers
  into a `*_helpers.go` (marginal value, splits the unit, does not achieve the goal). Not recommended
  alone — same verdict as SV2 Option C.

Stays BLOCKED on the founder choosing A / B / C for the whole agent package. The `depends_on:
[ARCHSV2]` is a *sequencing* link (avoid overlapping diffs) — but with SV2 deferred there is no diff
to sequence behind; SV3 is independently blocked on its own verified cycle, not on SV2 landing.

## Goal
Move the connector crawl-result ingestion cluster (crawl_ingest*.go) into a bounded `crawl_ingest/` subpackage.

## Component / domain
internal/server/agent crawl-result ingestion pipeline.

## Files likely involved
crawl_ingest.go (167), crawl_ingest_processor.go (83), crawl_ingest_types.go (58), crawl_ingest_item.go (96) → internal/server/agent/crawl_ingest/; caller crawl.go updates.

## Dependencies
ARCHSV2 (sequential agent-package moves to avoid overlapping diffs).

## Risk notes
YELLOW move-only. Best-effort telemetry path (ingest errors logged, never block). Direct-post CAS lives in the processor — MOVE ONLY, do not alter CAS. Characterization tests (multiitem/paths/payload) must stay green.

## Validation
go test ./internal/server/agent/... ; ai_validate.sh

## Done criteria
crawl_ingest/ subpackage + facade; crawl.go updated; no cycle; characterization tests green; move-only.
