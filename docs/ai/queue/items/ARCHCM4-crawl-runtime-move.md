---
id: ARCHCM4
status: READY
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM-R1, ARCHCM-R2]
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHCM4 — Move crawl runtime/plan/scheduler out of cmd/scraper

## Goal
Relocate crawl_runtime.go (373) + crawl_scheduler.go (172) plan-assembly / account-resolution / connector-dispatch / scheduling out of the composition root into internal/crawler + internal/jobs.

## Component / domain
crawler runtime + job scheduling.

## Files likely involved
cmd/scraper/crawl_runtime.go, crawl_scheduler.go → internal/crawler/plan.go + internal/jobs/*.

## Dependencies
ARCHCM-R1 (account-scope consolidation), ARCHCM-R2 (crawl runtime semantics audit) — both must be approved first.

## Risk notes
RED-ADJACENT — crawler/jobhandler runtime + connector dispatch + fallback chains. Gate behind the two audits. The move itself must be behavior-preserving; if it requires changing dispatch/retry semantics → STOP (RED).

## Validation
go build ./... ; go test ./... ; ai_validate.sh ; scripts/check_topology.sh

## Done criteria
crawl runtime/scheduler in owning packages; cmd only wires; dispatch/retry semantics identical; guards green.
