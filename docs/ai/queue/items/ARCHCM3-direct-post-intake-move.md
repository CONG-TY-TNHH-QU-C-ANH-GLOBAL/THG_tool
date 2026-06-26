---
id: ARCHCM3
status: READY
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM2, ARCHST-R3]
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHCM3 — Move direct-post intake into internal/directpost

## Goal
Move the direct-post application service + scheduler + coordinator (direct_post_intake.go, direct_post_intake_scheduler.go, direct_link_comment.go) from cmd into the existing internal/directpost package.

## Component / domain
direct-post intake domain.

## Files likely involved
cmd/scraper/direct_post_intake*.go, direct_link_comment.go → internal/directpost/.

## Dependencies
ARCHCM2 (intake calls the outbound queue facade); ARCHST-R3 (direct-post boundary audit must settle leads↔coordination ownership first).

## Risk notes
YELLOW move-only; scheduler state machine is domain logic, not infra. Preserve DP status transitions exactly. Account-guard duplication resolved via ARCHCM-R1.

## Validation
go build ./... ; go test ./... ; ai_validate.sh

## Done criteria
direct-post service/scheduler in internal/directpost; cmd only wires the goroutine; transitions unchanged; move-only.
