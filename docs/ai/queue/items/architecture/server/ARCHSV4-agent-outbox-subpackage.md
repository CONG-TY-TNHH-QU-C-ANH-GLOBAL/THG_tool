---
id: ARCHSV4
status: BLOCKED
lane: RED
risk: RED
depends_on: [ARCHSV2]
parallel_safe: false
branch: ""
pr_url: ""
blocked_on: human-boundary-decision
boundary_target: transport-to-usecase
---

# ARCHSV4 — Extract internal/server/agent/outbox subpackage

## Feasibility (VERIFIED 2026-06-29 — move-only is NOT possible; reclassified YELLOW→RED)
Same Handler-coupling / import-cycle blocker as [ARCHSV2](ARCHSV2-agent-finalize-subpackage.md) and
[ARCHSV3](ARCHSV3-agent-crawl-ingest-subpackage.md). The outbox cluster is worse-coupled than the
others — most files are HTTP transport handlers:

- `*Handler` methods taking `c *fiber.Ctx` (transport in the cluster): `agentGetOutbox`,
  `agentOutboxSent`, `agentOutboxFailed` (outbox_agent.go), `getOutbox`, `draftOutbound`,
  `editOutbound`, `deleteOutbound`, `deleteAllOutboundComments/Posts`, `requireOutboundOwnerRow`,
  `clearActorBlock` (outbox_dashboard.go), `agentOutboxPreSubmitVerify` (outbox_presubmit.go).
- `claimCandidate` (outbox_claim.go) is a `*Handler` method on the **execution_id CAS claim→finalize**
  flow — a controlled zone (the item's own risk note: CAS flow unchanged).
- outbox_dashboard.go serializes an operator-facing response whose **wire shape must stay byte-
  identical** (a DTO/wire change would be RED).

**Import-cycle blocker:** moving outbox_*.go to `outbox/` makes `outbox` import `agent` (for `*Handler`
+ the `finalize` orchestration the dep note cites) while `agent` registers these handlers on its routes
→ an `agent ↔ outbox` cycle, and it would drag `fiber` transport into the domain subpackage. Same
§4.3 founder-gated DI-port requirement as SV2/SV3.

## Decision
Folded into the **unified SV2+SV3+SV4 agent-package boundary decision** — see
[ARCHSV3](ARCHSV3-agent-crawl-ingest-subpackage.md) §"Unified boundary decision" (Option A defer all /
B one DI-port refactor / C GREEN pure-helper sub-slices). Stays BLOCKED on that founder choice. The
`depends_on: [ARCHSV2]` is sequencing only; SV4 is independently blocked on its own verified cycle.

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
