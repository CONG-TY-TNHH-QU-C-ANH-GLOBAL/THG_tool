---
id: ARCHCM2d
status: DONE
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM2b, ARCHCM2c]
parallel_safe: false
branch: "chore/archcm2d-facade-queue-port"
pr_url: https://github.com/CONG-TY-TNHH-QU-C-ANH-GLOBAL/THG_tool/pull/182
blocked_on: ""
boundary_target: transport-to-usecase
---

# ARCHCM2d — Reduce outbound_action_queueing.go to a thin cmd facade

## DONE (2026-06-30) — facade holds no direct queue write
On branch `chore/archcm2d-facade-queue-port`. After ARCHCM2c moved the lead-outreach
spine to `internal/services/facebook/leadoutreach`, `queueLeadOutreach` was already a thin
facade (resolve context → readiness gate → fetch leads → `leadoutreach.New`/`ProcessLead`/
`FormatResult`). The one remaining piece of composition-root business logic was
`queueFacebookPostTargets`'s direct `db.QueueOutboundForOrg` write loop (also the *deprecated*
wrapper). This slice routes that write through the existing, proven `leadoutreach.OutboundRecorder`
(`storeOutboundRecorder`, already 1:1-verified in seam 1) — the post path already imported
`leadoutreach` for `Mode`. Effect:
- **No direct queue write left in `outbound_action_queueing.go`** — all four `queue*` entry
  points are now thin arg-parse + context/target-resolution + delegate adapters.
- Fixes the deprecated `db.QueueOutboundForOrg` call (now the non-deprecated `Outbound().Queue`
  via the recorder).
- Behavior-preserving: same `OutboundMessage` payload / 24h cooldown / `Allowed`+`ExecutionState`
  checks; `QueueOutbound` maps the store `Decision` 1:1 (`result.Decision.Allowed` → `result.Allowed`).

Closes the ARCHCM2 umbrella's facade goal. NOTE: ARCHCM3 (`depends_on: [ARCHCM2, ARCHST-R3]`)
stays blocked on the RED `ARCHST-R3` even once the umbrella closes — it does not free on this PR.
Mild residual smell (non-blocking): the shared outbound primitives `Mode`/`OutboundRecorder`/
`QueueOutcome` live in `leadoutreach` but are reused by the FB-post path; a future neutral
`fboutbound` home could host them if the posting cluster grows — not worth a speculative package now.

## Goal
Once the L3 core lives in `internal/outbound` (ARCHCM2b + ARCHCM2c), reduce L1
(`outbound_action_queueing.go`, the `queue*` entry points holding 18/19 of the
cluster's `arg*` calls) to a thin composition-root facade: parse `args map[string]any`,
resolve execution context (L2, per ARCHCM2a), then delegate to `internal/outbound`.

## Component / domain
outbound action queueing entry points (composition-root adapter layer).

## Scope
- `queueLeadOutreach`, `queueGroupPost`, `queueProfilePost`, `queueFacebookPostTargets`
  stay in cmd as thin arg-parsing adapters over `outbound.*`.
- External callers (`facebook_account_scope.go`, direct-link/direct-post intake,
  crawl) keep calling the cmd entry points (no facade churn for them) OR switch to
  `outbound.*` where cleaner — decide during the slice.

## Dependencies
ARCHCM2b (package established) and ARCHCM2c (lead pipeline/outcome moved).

## Risk notes
YELLOW: the entry points orchestrate the outbound queue path — preserve queue/dedup/
policy semantics EXACTLY. Behavior-preserving; this is the final cleanup that closes
the ARCHCM2 umbrella.

## Validation
go build/test ./... ; scripts/check_topology.sh ; ai_validate.sh.

## Done criteria
`outbound_action_queueing.go` is a thin cmd facade over `internal/outbound`; no
business logic left in the composition root for this cluster; queue semantics
identical; guards green. On merge, the ARCHCM2 umbrella → DONE (then ARCHCM3 unblocks).
