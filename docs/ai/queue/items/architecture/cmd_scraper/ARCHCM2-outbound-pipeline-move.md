---
id: ARCHCM2
status: READY
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM1, ARCHCM-R1]
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHCM2 — Move outbound pipeline out of cmd/scraper into internal/outbound

## Goal
Relocate the leaked outbound business domain (outbound_action_queueing/lead_pipeline/lead_outcome/action_context/comment_reasoning, ~724 LOC) from the composition root into the owning internal/outbound package behind a facade.

## Component / domain
outbound domain. Moves logic OUT of cmd into internal/outbound.

## Files likely involved
cmd/scraper/outbound_*.go → internal/outbound/*; cmd callers (agent_actions.go, skills_register.go) switch to outbound.* facade.

## Dependencies
ARCHCM1 (pure-helper split first); ARCHCM-R1 (account-scope consolidation — shared RBAC the pipeline calls).

## Risk notes
YELLOW move-only but touches outbound queue path — preserve queue/dedup/policy semantics EXACTLY (queue writes are RED if altered). Behavior-preserving move; tests migrate to internal/outbound.

## Validation
go build ./... ; go test ./... ; ai_validate.sh ; scripts/check_topology.sh

## Done criteria
outbound_*.go gone from cmd; internal/outbound exposes a clean facade; callers updated; topology/tenant guards green; no queue-semantics change.
