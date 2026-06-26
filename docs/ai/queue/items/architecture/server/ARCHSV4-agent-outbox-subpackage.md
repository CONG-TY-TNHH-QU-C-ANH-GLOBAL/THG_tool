---
id: ARCHSV4
status: READY
lane: YELLOW
risk: YELLOW
depends_on: [ARCHSV2]
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHSV4 — Extract internal/server/agent/outbox subpackage

## Goal
Move the outbound claim/presubmit/dashboard cluster (outbox_*.go) into a bounded `outbox/` subpackage.

## Component / domain
internal/server/agent outbound claim + presubmit verification endpoints.

## Files likely involved
outbox_agent.go (146), outbox_claim.go (46), outbox_presubmit.go (81), outbox_dashboard.go (218) → internal/server/agent/outbox/; routes.go updates.

## Dependencies
ARCHSV2 (outbox calls finalize; finalize subpackage settles first).

## Risk notes
YELLOW move-only. outbox_dashboard.go serializes an operator-facing response — preserve the response shape EXACTLY (no DTO/wire change; that would be RED). execution_id CAS claim→finalize flow unchanged.

## Validation
go test ./internal/server/agent/... ; ai_validate.sh

## Done criteria
outbox/ subpackage + facade; routes updated; dashboard JSON byte-identical; CAS flow unchanged; move-only.
