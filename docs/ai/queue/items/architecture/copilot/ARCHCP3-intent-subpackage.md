---
id: ARCHCP3
status: READY
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCP1, ARCHCP2]
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHCP3 — Extract copilot/intent subpackage

## Goal
Move the pure intent-classification cluster (intent_normalize/lexicon/types/entities/router) into a bounded `copilot/intent/` subpackage with a small facade (DeterministicFacebookAction, RouteDecisionFor).

## Component / domain
internal/drivers/copilot intent classification.

## Files likely involved
intent_*.go (+ intent_*_test.go) → internal/drivers/copilot/intent/; agent.go / agent_action_router.go / agent_preflight.go call sites updated to intent.* facade.

## Dependencies
ARCHCP1, ARCHCP2 (reduce in-package complexity before moving boundaries; mirrors the internal/ai staged precedent).

## Risk notes
YELLOW move-only. Verified ZERO cross-cluster cycle: intent_* references no agent_* symbols; agent→intent is one-way. Blast radius ~5 internal call sites; external importers (server, cmd) call Agent, not intent helpers.

## Validation
go build ./... ; go vet ./internal/drivers/copilot/... ; go test ./... ; ai_validate.sh

## Done criteria
copilot/intent/ exists with files + doc + tests moved; copilot package imports ./intent via facade; no cycle; behavior identical; move-only.
