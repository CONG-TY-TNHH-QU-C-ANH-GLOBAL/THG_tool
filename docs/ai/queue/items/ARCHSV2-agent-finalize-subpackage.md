---
id: ARCHSV2
status: READY
lane: YELLOW
risk: YELLOW
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHSV2 — Extract internal/server/agent/finalize subpackage

## Goal
The agent package is a 49-file flat package with a 13×crawl_/5×finalize_/4×outbox_ prefix smell. Move the outbound finalization cluster (finalize_outbound.go, finalize_side_effects.go, finalize_helpers.go) into a bounded `finalize/` subpackage with a small facade.

## Component / domain
internal/server/agent finalization state-machine + side effects.

## Files likely involved
finalize_outbound.go (190), finalize_side_effects.go (191), finalize_helpers.go (186) → internal/server/agent/finalize/; caller outbox_agent.go updates to the facade.

## Dependencies
None, but YELLOW sequential. Move-only, behavior-preserving.

## Risk notes
YELLOW / CAS-adjacent — finalize calls store.FinalizeOutboundAttempt (lease/execution_id idempotency). MOVE-ONLY, no semantic change; verify idempotency-replay test still passes. Do NOT alter the CAS gate. If a move needs exporting private ledger internals → STOP (RED).

## Validation
go test ./internal/server/agent/... ; go vet ; ai_validate.sh

## Done criteria
finalize/ subpackage with package doc + facade; callers updated; no import cycle; idempotency tests green; move-only diff.
