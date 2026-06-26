---
id: ARCHCP4
status: READY
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCP3]
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHCP4 — (Optional) Extract copilot/agent subpackage

## Goal
After intent/ is stable, optionally move the agent orchestration cluster into copilot/agent/. Only do this if the flat package still trips the >15-file trigger after ARCHCP1-3.

## Component / domain
internal/drivers/copilot agent orchestration.

## Files likely involved
agent.go + agent_*.go + brain_*.go (post-split) + tests → internal/drivers/copilot/agent/.

## Dependencies
ARCHCP3.

## Risk notes
YELLOW move-only, larger blast radius (server/cmd import the agent entrypoints). Re-evaluate necessity first — agent/ staying flat at <15 files is acceptable. Do not move speculatively.

## Validation
go build ./... ; go test ./... ; ai_validate.sh

## Done criteria
Either a clean move-only extraction with all importers updated, OR a documented decision to leave flat. No behavior change.
