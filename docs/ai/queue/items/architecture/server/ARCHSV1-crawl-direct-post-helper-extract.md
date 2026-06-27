---
id: ARCHSV1
status: DONE
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: true
branch: "chore/archsv1-direct-post-outcome-extract"
pr_url: https://github.com/CONG-TY-TNHH-QU-C-ANH-GLOBAL/THG_tool/pull/134
---

# ARCHSV1 — Extract crawl_direct_post outcome-classification helpers

## Goal
Pull the deterministic outcome/retry/terminal-failure classification out of crawl_direct_post.go + crawl_direct_post_failure.go into a pure sibling helper, reducing handler complexity.

## Component / domain
internal/server/agent — direct-post crawl outcome classification.

## Files likely involved
internal/server/agent/crawl_direct_post.go, crawl_direct_post_failure.go, NEW crawl_direct_post_outcome.go (same package).

## Dependencies
None (parallel-safe; disjoint from finalize/outbox items).

## Risk notes
GREEN — pure deterministic classification (no DB/IO), same package, no DTO/wire change. Note S107 on crawl_direct_post.go:38 may also be addressed via a param struct (keep behavior identical).

## Validation
go test ./internal/server/agent/... ; ai_validate.sh

## Done criteria
Classification helpers in their own pure file with direct tests; both call sites use them; no behavior change; no response-shape change.
