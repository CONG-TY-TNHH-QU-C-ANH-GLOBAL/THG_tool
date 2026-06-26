---
id: ARCHCP2
status: READY
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: true
branch: ""
pr_url: ""
---

# ARCHCP2 — Split agent_preflight.go (readiness vs business-context)

## Goal
agent_preflight.go (342) mixes account/browser readiness with business-calibration/targeting inference. Split into agent_account_readiness.go and agent_business_context.go.

## Component / domain
internal/drivers/copilot preflight. Package `copilot`.

## Files likely involved
agent_preflight.go → agent_account_readiness.go / agent_business_context.go (same package); keep noop facades that agent.go calls.

## Dependencies
None (parallel-safe with ARCHCP1).

## Risk notes
GREEN — same package, pure extraction. The two concerns don't call each other (verified no cross-couple). Preserve facade names agent.go depends on.

## Validation
go test ./internal/drivers/copilot/... ; ai_validate.sh

## Done criteria
Both siblings < 200 lines; agent.go entry-points unchanged; tests green; no behavior change.
