---
id: ARCHSV-R1
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHSV-R1 — AUDIT: workspace browser-orchestration split

## Goal (audit-only)
internal/server/workspace watchers.go (655) + handlers.go (644) are browser-lifecycle runtime-state machines. Decide whether to split into lifecycle + proxy subpackages.

## Component / domain
internal/server/workspace browser orchestration. RED (runtime state).

## Files likely involved
watchers.go, handlers.go, screen_proxy.go, vnc_proxy.go; spec specs/WORKSPACE_BROWSER_LIFECYCLE.md.

## Dependencies
None.

## Risk notes
RED — browser instance lifecycle mutations (BrowserState/CDPPort/VNCPort) affect concurrency; auth/session-adjacent (auth/onboarding.go cookie writes are also RED). No autonomous split. Human decision after runtime-state review.

## Validation
N/A (audit).

## Done criteria
Decision record: split plan + concurrency-safety verification points, or defer with rationale.
