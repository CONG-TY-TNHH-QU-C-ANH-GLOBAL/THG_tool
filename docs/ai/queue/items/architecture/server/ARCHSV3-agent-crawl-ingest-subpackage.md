---
id: ARCHSV3
status: READY
lane: YELLOW
risk: YELLOW
depends_on: [ARCHSV2]
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHSV3 — Extract internal/server/agent/crawl_ingest subpackage

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
