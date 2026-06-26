---
id: ARCHCP1
status: REVIEW
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: true
branch: chore/arch-cp1-agent-brain-split
pr_url: ""
---

# ARCHCP1 — Split agent_brain.go into responsibility siblings

## Goal
Decompose the 525-line god-file agent_brain.go (3 responsibilities: BrainClient HTTP transport, Brain DTO types, plan validation + action-arg prep) into sibling files, leaving a thin processBrainPlan orchestrator.

## Component / domain
internal/drivers/copilot agent brain. Package `copilot`.

## Files likely involved
agent_brain.go → brain_client.go / brain_types.go / brain_plan_validator.go / brain_action_prep.go (same package); agent_brain.go keeps the orchestrator.

## Dependencies
None (parallel-safe with ARCHCP2; disjoint files).

## Risk notes
GREEN — same package, no import-boundary change, pure extraction. Verify each new helper is itself under the S3776 threshold (do not relocate complexity wholesale). agent_brain_test.go guards behavior.

## Validation
go test ./internal/drivers/copilot/... ; go vet ; ai_validate.sh

## Done criteria
agent_brain.go < 200 lines; each sibling < 200; tests green; no behavior change; no exported-symbol churn beyond what same-package move requires (none).
