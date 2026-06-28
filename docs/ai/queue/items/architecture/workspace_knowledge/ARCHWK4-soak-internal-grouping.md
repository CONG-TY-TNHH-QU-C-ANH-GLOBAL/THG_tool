---
id: ARCHWK4
status: READY
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: false
branch: "chore/archwk4-batch-d-fixtures-split"
pr_url: ""
boundary_target: prep-extraction
last_batch: D-fixtures
---

# ARCHWK4 — Soak package internal decomposition

## Goal
The `soak` package has 12 source files spanning 4 internal layers (harness orchestration / reporting / evaluation+fixtures / test-doubles). Decide between sibling-file grouping (GREEN) vs bounded subpackages soak/report, soak/eval, soak/doubles (YELLOW import-boundary).

## Component / domain
KnowledgeOS soak evaluation harness. Package `soak`.

## Files likely involved
report.go (477), harness.go (397), fixtures.go (311), failure_modes.go (237), embedder.go (210), gold_dataset.go/gold_eval.go/precision.go, fake_openai.go, semantic_searcher.go.

## Dependencies
None, but sequential (single package, large diff). Soak is internal-only (no external importers) so subpackaging is low blast-radius.

## DESIGN DECISION (2026-06-27): GREEN sibling-file grouping — NOT subpackages
Verified facts: all 17 files are `package soak`; **zero external importers** (the
`internal/workspace_knowledge/soak` hit in embedding/openai.go is a doc comment, not
an import); soak is test-support, not a runtime hot path.

Decision: **sibling-file re-grouping within `package soak`** (GREEN, no import change),
NOT `soak/report` `soak/eval` `soak/doubles` subpackages. Rationale: subpackaging adds
import-boundary churn + YELLOW risk for **zero** external benefit (nothing outside soak
imports it); a test harness does not earn package boundaries. Matches the
freeze-abstraction lean (runtime is the focus). `boundary_target: prep-extraction`,
lane reclassified YELLOW→GREEN.

## Staged batch plan (multi-batch — one god-file per PR; stays READY)
Each god-file needs a 3-way split or a documented large-data/cohesion exception — a
2-file split leaves a >200 sibling (measured), so this is staged, not big-bang. Seam
analysis (verified via top-level decl scan):

- **Batch A — report.go (477) — DONE (this PR):** found a 3rd responsibility
  (`computeQualityAggregates`/`mean`/`percentile` = aggregation, not rendering). Split
  into 4 files, all <200, bodies moved verbatim: `report.go` (data-model types, 195),
  `report_markdown.go` (ToMarkdown + summary sections, 127), `report_markdown_detail.go`
  (compliance/failure-mode/real-soak detail, 94), `report_metrics.go` (ToJSON +
  aggregation, 84). Markdown verified byte-identical (only the `Generated:` timestamp +
  pre-existing `AssetsByType` map-iteration order vary). report.go dropped off the
  allowlist.
- **Batch B — harness.go (397) — DONE:** contiguous 3-way split, all <200, bodies
  sed-extracted verbatim (imports auto-fixed with pinned goimports): `harness.go`
  (Harness type + Run orchestrator, 136), `harness_pipeline.go` (ingest/embed/
  buildSearcher/setupSource/runOnePrompt stage steps, 154), `harness_metrics.go`
  (detectFallback/scanHitSignals/soakVerdict/measureReplayHealth/measureStale/
  isTraceComplete, 132). harness.go dropped off the allowlist. soak tests green;
  artifact diffs are only non-deterministic runtime values (timestamp, map order,
  measured latencies) — structure/scores/verdicts identical.
- **Batch C — failure_modes.go (237) — DONE:** moved the test-doubles
  (`brokenEmbedder`, `slowSearcher`) into `soak_doubles.go` (46); the
  `runFailureModes` dispatcher + 6 `failureMode*` scenario methods stay in
  `failure_modes.go` (198, after condensing one verbose doc comment — no scenario
  sub-split needed). Both <200; failure_modes.go dropped off the allowlist. soak
  tests green; doubles are same-package so no behavior change.
- **Batch D — fixtures.go (311) — DONE:** split into `lead_fixtures.go`
  (`LeadPrompt` + `RealisticLeads`, 112, no imports — pure literals) and
  `catalog_fixtures.go` (`CatalogFixture` + `RealisticCatalog` + `PayloadJSON`, 200).
  Catalog landed exactly at the 200 limit (curated data literal), so NO allowlist
  entry was needed; the old `fixtures.go` (allowlisted 311) was deleted + removed
  from the allowlist. soak tests green; same package, no behavior change.
- **Batch E — embedder.go (210):** cohesive deterministic test embedder, only 10 over.
  Low value to split; prefer a documented large-fixture exception unless `tokenise`
  extraction earns its own file.

This PR records the decision only (no code). Each batch above is a clean GREEN
follow-up under `BOUNDARY_MIGRATION_PLAYBOOK.md` §3.

## Risk notes
YELLOW if subpackages (import boundaries); GREEN if pure sibling re-grouping. Soak is test-support, not a runtime hot path. Confirm no import cycle (soak imports retrieval/embedding/assets only).

## Validation
go test ./internal/workspace_knowledge/soak/... ; ai_validate.sh

## Done criteria
Each soak file maps to one responsibility; no god-files >200 added; behavior preserved; design (sibling vs subpackage) recorded in the PR body.
