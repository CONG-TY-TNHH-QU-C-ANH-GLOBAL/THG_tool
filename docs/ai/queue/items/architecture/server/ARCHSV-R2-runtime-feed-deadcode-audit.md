---
id: ARCHSV-R2
status: REVIEW
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: false
branch: "arch/archsv-r2-delete-dead-runtime-feed"
pr_url: ""
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

## Resolution (architect-sprint — Option A: delete, self-approved §4.1)

Re-verified the evidence against HEAD before acting:
- `runtimeFeed`, `buildRuntimeFeedRow`, `runtimeFeedRow` are referenced ONLY inside
  `runtime_feed.go`; no route registers `runtime-feed` (grep in `routes.go` empty);
  no test references them. The whole file is dead.
- `coordination.ListRecentRuntimeEvents` has exactly ONE production consumer — the dead
  handler (otherwise only its own definition + test). After this delete it is
  consumer-less.

**Decision: Option A (delete `runtime_feed.go`).** This is the only option self-approvable
under Autonomy v2 §4.1 because it **preserves current behavior** — the handler is
unreachable today (no route), so removing it changes no runtime behavior and clears the
OPEN go:S3776 on `runtimeFeed`. **Option B (wire the route) was NOT taken**: registering a
new reachable `GET /api/observability/runtime-feed` endpoint *adds* product-visible
behavior (a new data path), which §4.3 reserves for a founder decision. If the runtime-feed
dashboard is wanted, re-open as a behavior-adding feature item.

**`ListRecentRuntimeEvents` left in place (noted, not deleted):** it sits in
`internal/store/coordination` (RED-adjacent store layer) and still has passing tests, so it
does not trip the unused-code guard. Deleting it would expand this single-root GREEN PR into
the store layer for no behavior gain. Tracked follow-up (GREEN): remove
`ListRecentRuntimeEvents` + `runtime_events_test.go`'s coverage of it, OR keep it as
coordination read-API surface — a deliberate keep-vs-remove call, not bundled here.

Item stays `REVIEW` until the PR merges; DONE is set only by queue reconcile.
