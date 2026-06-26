---
id: PR31D
status: DONE
lane: GREEN
risk: YELLOW -> GREEN/test-only
depends_on: []
parallel_safe: false
branch: refactor/pr31d-facebook-crawl-session-seam
pr_url: ""
---

# PR31D — Facebook crawl session fake seam

## Goal

Introduce the smallest fakeable seam for the session-acquire branch if needed.

## Result

Completed as test-only. Existing seam reused. No production seam introduced.

## Scope

- internal/jobhandlers/facebook_crawl
- internal/runtime
- internal/livesession

## Constraints

- no real Chrome/browser/CDP/network in tests
- no broad Browser framework
- no package moves

## Validation

- scripts/ai_preflight.sh
- scripts/ai_validate.sh

## Notes

Merged via PR #113.
