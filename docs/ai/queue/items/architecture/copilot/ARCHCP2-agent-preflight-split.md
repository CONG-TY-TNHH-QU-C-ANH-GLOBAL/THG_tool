---
id: ARCHCP2
status: REVIEW
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: true
branch: "chore/archcp2-agent-preflight-split"
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
GREEN — same package, pure extraction. The two concerns don't call each other (verified no cross-couple). Preserve facade names agent.go depends on. Note: business-context exceeded 200 lines on its own, so it was split again into agent_business_context.go (persisted calibration capture) + agent_crawl_targeting.go (ephemeral prompt-scoped crawl signals) to satisfy the file-size guard. All callers are in-package, so no facades were needed.

## Validation
go test ./internal/drivers/copilot/... ; ai_validate.sh

## Done criteria
Both siblings < 200 lines; agent.go entry-points unchanged; tests green; no behavior change.
