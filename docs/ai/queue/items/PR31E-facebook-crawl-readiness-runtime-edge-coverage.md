---
id: PR31E
status: DONE
lane: YELLOW
risk: GREEN/YELLOW
depends_on: [PR31D]
parallel_safe: false
branch: test/pr31e-crawl-readiness-runtime-edge-coverage
pr_url: ""
---

# PR31E — Facebook crawl readiness/runtime edge coverage

## Goal

Add missing characterization tests around not-ready/offline/human_required/failure mapping if existing seams allow.

## Scope

- internal/jobhandlers/facebook_crawl
- internal/readiness
- cmd/scraper crawl/readiness tests

## Constraints

- test-only preferred
- no runtime semantics change
- no real Chrome/browser/CDP/network
- use existing fakes/helpers first
- if a missing seam requires broad production signature changes, stop and report

## Validation

- go test ./internal/jobhandlers/... -run 'Facebook|Crawl|Session|Runtime|Human|Offline|Readiness' -v
- scripts/ai_preflight.sh
- scripts/ai_validate.sh

## Result

Merged via PR #115.
