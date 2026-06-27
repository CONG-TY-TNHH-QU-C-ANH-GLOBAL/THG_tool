---
id: ARCHWK3
status: DONE
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: true
branch: "chore/archwk3-retrieval-helpers-rename"
pr_url: https://github.com/CONG-TY-TNHH-QU-C-ANH-GLOBAL/THG_tool/pull/145
boundary_target: prep-extraction
---

# ARCHWK3 — Rename retrieval/helpers.go into responsibility files

## Goal
Replace the vague `retrieval/helpers.go` catch-all (rule-5 smell) with responsibility-named files: tokenize.go (Tokenize), scoring.go (Clamp01/BuildReason), query.go (TruncateQuery). RecordRejection stays with the Trace types.

## Component / domain
KnowledgeOS retrieval shared utilities. Package `retrieval`.

## Files likely involved
- internal/workspace_knowledge/retrieval/helpers.go (deleted)
- NEW tokenize.go / scoring.go / query.go

## Dependencies
None.

## Risk notes
GREEN but LOW VALUE — helpers.go is already <200 lines and cohesive; only justified to clear the vague-name smell. Do NOT over-fragment (no <10-line micro files unless the name earns it). Callers use `retrieval.X` (same package re-export) — no import change.

## Validation
go test ./internal/workspace_knowledge/retrieval/... ; ai_validate.sh

## Done criteria
No file named helpers.go in retrieval/; each util in a responsibility file; exported symbols unchanged; tests green.
