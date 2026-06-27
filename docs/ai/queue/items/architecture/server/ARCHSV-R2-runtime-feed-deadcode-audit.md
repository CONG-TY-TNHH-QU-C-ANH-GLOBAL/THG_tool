---
id: ARCHSV-R2
status: BLOCKED
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
blocked_on: human-decision
boundary_target: blocked-decision
---

# ARCHSV-R2 — AUDIT: unwired runtime_feed handler (dead code: delete vs wire)

## Goal (audit-only — decide, do not auto-fix)
`internal/server/observability/runtime_feed.go` defines a handler `runtimeFeed`
(plus `buildRuntimeFeedRow` + the `runtimeFeedRow` wire struct) that is **never
registered**. Decide whether to DELETE it or WIRE the missing route. Tracked here
deliberately as a separate item so it is NOT resolved inside Sonar cleanup.

## Evidence (verified 2026-06-27)
- `observability/routes.go::Routes` registers 10 sibling handlers
  (`executionDistribution`, `executionRecent`, … `promptRoutingMissingSignals`)
  but **not** `runtimeFeed`.
- The route the doc comment claims it serves — `GET /api/observability/runtime-feed`
  — is registered nowhere; a repo-wide grep for `runtimeFeed` / `runtime-feed`
  route wiring finds only comments and the store query `ListRecentRuntimeEvents`.
- `runtimeFeed`, `buildRuntimeFeedRow`, and the `runtimeFeedRow` type are referenced
  only within `runtime_feed.go` itself → the whole file is unreferenced dead code.
- gopls flags `runtimeFeed` as unused; it carries an OPEN go:S3776 (cognitive
  complexity) — which is why it surfaced during a Sonar YELLOW pass. It was
  intentionally **left untouched** there (do not polish or delete unverified-intent
  code inside a Sonar batch).

## Decision needed (founder)
- **Option A — delete** `runtime_feed.go` (and confirm `coordination.ListRecentRuntimeEvents`
  has no other consumer; if it too is dead, note it). Removes the S3776 + dead code.
- **Option B — wire** the route: add `group.Get("/observability/runtime-feed", runtimeFeed(deps))`
  in `routes.go` (the handler is org-scoped + read-only, matching the package's GET-only
  contract) and reduce the S3776 as part of finishing the feature.

Either option is GREEN/low-risk (no behavior change today — the code is currently
unreachable). Once decided, unblock and implement in one bounded PR.

## Validation
go build ./... ; go vet ./internal/server/observability/... ; ai_validate.sh

## Done criteria
runtime_feed.go is either removed or its route is registered + S3776 resolved; no
unreferenced handler remains; ai_validate green.
