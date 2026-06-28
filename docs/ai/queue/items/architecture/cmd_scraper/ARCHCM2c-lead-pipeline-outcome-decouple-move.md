---
id: ARCHCM2c
status: BLOCKED
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM2a, ARCHCM2b]
parallel_safe: false
branch: ""
pr_url: ""
blocked_on: cmd-local-helper-decoupling
boundary_target: transport-to-usecase
---

# ARCHCM2c — De-couple + move outbound_lead_outcome.go / outbound_lead_pipeline.go

## Goal
Move the remaining L3 files — `outbound_lead_outcome.go` and the
`outbound_lead_pipeline.go` orchestration spine — into `internal/outbound`, after
de-coupling them from the cmd-local helpers they reach into.

## Component / domain
outbound lead pipeline + outcome recording.

## Blockers (why this is not yet READY)
Unlike comment_reasoning, these two files are glued to cmd-local helpers defined
outside the cluster:
- lead_outcome: `formatCommentResult`, `formatOutreachResult`,
  `noEligibleCommentMessage`, `queueOutreachMessage`, `recordSkip`.
- lead_pipeline: `coverageGate`, `businessContextForOrg`, `queueOutreachMessage`,
  `recordSkip`, `formatOutreachResult`, `prepareOutreachContent`, `processOutreachLead`,
  `fbContactDirectory`, plus one stray `argString(args,"template")`.

Each must be lifted into the cluster, injected, or relocated to a shared package
before the move, without changing behavior. lead_pipeline also calls into the L2
resolution layer via the queue facade, so its final shape depends on the L2 home
(ARCHCM2a) and on the `internal/outbound` package established by ARCHCM2b.

## Dependencies
ARCHCM2a (L2 home decided), ARCHCM2b (package + facade established).

## Risk notes
YELLOW move touching the outbound queue *call sites* (`QueueOutboundForOrg`,
`RecordOutcome`) — preserve queue/dedup/policy semantics EXACTLY (queue writes are RED
if altered). Behavior-preserving; characterization before each helper decouple.

## Validation
go build/test ./... ; scripts/check_topology.sh ; ai_validate.sh.

## Done criteria
lead_outcome + lead_pipeline in `internal/outbound`; shared cmd helpers decoupled;
no import cycle; queue semantics identical; guards green.
