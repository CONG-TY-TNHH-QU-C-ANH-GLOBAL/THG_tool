---
id: ARCHSV4
status: REVIEW
lane: YELLOW
risk: YELLOW
depends_on: [ARCHSV2]
parallel_safe: false
branch: "refactor/extract-agent-outbox-subpackage"
pr_url: ""
boundary_target: leaf-move
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

## RESOLUTION (Architecture Convergence Mode — PR2 of the staged outbound-execution split)

Done on top of the merged finalize boundary (ARCHSV2). The clean boundary is the
**6-file set** {outbox_agent, outbox_claim, outbox_presubmit, outbox_dashboard,
comment_verify, reverify} → `internal/server/agent/outbox` (self-Handler pattern; the
package never imports agent → no cycle). `comment_verify`+`reverify` were pulled in
because they share `requireOutboundOwnerRow` with the dashboard (reverse-coupling) — the
6-set is the minimal self-contained unit; splitting would break them.

Two consumer-owned ports (founder-sanctioned, "where they unlock a real move"):
- `outboxReadyNotifier` (`NotifyOutboxReady(int)`) — the dashboard's sole `*WSHub` use.
- `accountOwnerGuard` — `RequireAccountOwner` STAYS in agent (also used by
  `server/workspace`); injected as a func value so outbox needn't import agent.
Outbox delegates the terminal step to `*finalize.Handler` (outbox → finalize).

Behavior byte-for-byte / move-only: execution_id CAS (`claimCandidate`), claim/lease/
idempotency, action_ledger semantics (topology [6] baseline 2 unchanged), and the
dashboard list **wire shape** all moved verbatim. Routes preserved exactly via
`outbox.RegisterConnectorRoutes` (token auth) + `outbox.RegisterDashboardRoutes`
(tenant/adminOnly) — same paths/auth/order. `agent.Handler` dropped its now-unused
`finalize` field (outbox owns it). The 218-line `outbox_dashboard.go` was split (size
guard) into `outbox_dashboard.go` (read/draft) + `outbox_mutations.go` (owner-check +
edit/delete) — same package, behavior unchanged.

Tests: the outbox-handler integration tests (claim / idempotent-replay / stale-
execution_id / dashboard) moved into `package outbox` and pass unchanged;
`account_guard_test` stayed in agent (tests `AccountOwnerAllowed`, which stayed).

This completes the staged agent outbound-execution split (SV2 finalize + SV3
crawl-ingest + SV4 outbox). The flat agent god-package is materially reduced.
