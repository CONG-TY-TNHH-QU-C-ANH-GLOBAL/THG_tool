---
id: ARCHWK4
status: READY
lane: YELLOW
risk: YELLOW
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
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

## Risk notes
YELLOW if subpackages (import boundaries); GREEN if pure sibling re-grouping. Soak is test-support, not a runtime hot path. Confirm no import cycle (soak imports retrieval/embedding/assets only).

## Validation
go test ./internal/workspace_knowledge/soak/... ; ai_validate.sh

## Done criteria
Each soak file maps to one responsibility; no god-files >200 added; behavior preserved; design (sibling vs subpackage) recorded in the PR body.
