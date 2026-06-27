---
id: ARCHWK4
status: READY
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: false
branch: "chore/archwk4-design-decision"
pr_url: ""
boundary_target: prep-extraction
last_batch: design-decision
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

- **Batch A — report.go (477):** split `report.go` (the Report + 11 metric sub-structs
  = the data model, ~190) from the markdown rendering (`ToMarkdown` + 9 `write*` section
  methods, ~275). The rendering is ONE cohesive responsibility with no clean <200
  sub-seam → either accept `report_markdown.go` as a cohesive renderer (allowlist with
  justification) or split the `write*` methods into summary vs detail sections. Decide in
  the batch PR. **Do NOT change the emitted markdown** (it drives the
  `RETRIEVAL_SOAK_REPORT.md` artifact — behavior-locked).
- **Batch B — harness.go (397):** keep `Harness` + `Run` + ingest/embedding/searcher/
  prompt orchestration; move the pure scoring free-funcs (`detectFallback`,
  `scanHitSignals`, `soakVerdict`, `isTraceComplete`) + `measureReplayHealth`/
  `measureStale` into `harness_metrics.go`. May still need a 3rd file to land <200.
- **Batch C — failure_modes.go (237):** move the test-doubles (`brokenEmbedder`,
  `slowSearcher`) into `soak_doubles.go`; the 7 `failureMode*` scenario methods stay.
  (~207 after — likely needs one scenario sub-split or a documented exception.)
- **Batch D — fixtures.go (311):** split `LeadPrompt` + `RealisticLeads()` into
  `lead_fixtures.go` (~95). `CatalogFixture` + `RealisticCatalog()` is a ~150-line
  HAND-TUNED curated catalog literal = **intentionally large test data** → allowlist
  `catalog_fixtures.go` (~210) with that justification rather than fragmenting the data.
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
