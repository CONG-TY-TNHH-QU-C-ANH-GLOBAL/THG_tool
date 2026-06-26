---
id: ARCHWK1
status: REVIEW
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: true
branch: chore/arch-epic-wk1-governance-split
pr_url: ""
---

# ARCHWK1 — Split governance Layer-3 output validator into per-check files

## Goal
Decompose the `internal/workspace_knowledge/governance` god-file (output_validator.go, 329-line allowlist baseline) into one file per validation check, leaving a slim orchestrator + types. Pure file-responsibility cleanup.

## Component / domain
KnowledgeOS governance (Layer-3 output validation). Package `governance`.

## Files likely involved
- internal/workspace_knowledge/governance/output_validator.go (orchestrator + types stay)
- NEW banned_claim_check.go / guarantee_check.go / shipping_check.go / pricing_check.go (moved funcs+vars)
- scripts/file_size_allowlist.txt (remove entry once <200)

## Dependencies
None — first executable item of the epic.

## Risk notes
GREEN. Same package `governance`; no import-boundary change, no exported-symbol change, functions are pure. Existing output_validator_test.go (13 tests through ValidateOutput) guards behavior end-to-end.

## Validation
- scripts/ai_preflight.sh ; scripts/ai_validate.sh
- go test ./internal/workspace_knowledge/governance/...

## Done criteria
output_validator.go < 200 lines (orchestrator + ValidationVerdict/Reason/Code only); each check in its own <200-line sibling; allowlist entry removed; all existing tests green; no behavior change.
