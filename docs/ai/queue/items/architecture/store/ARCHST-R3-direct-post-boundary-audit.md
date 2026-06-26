---
id: ARCHST-R3
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHST-R3 — AUDIT: direct-post lookup boundary (leads vs coordination)

## Goal (audit-only)
leads/direct_post_lookup.go (lead-matching) and coordination/direct_post_workflow*.go (workflow state machine) appear to be one domain split across two packages. Confirm the boundary is correct or propose a move.

## Component / domain
store leads ↔ coordination cross-domain projection. RED.

## Files likely involved
leads/direct_post_lookup.go, coordination/direct_post_workflow*.go; spec specs/DIRECT_POST_INTAKE_WORKFLOW.md.

## Dependencies
Relates to ARCHCM3 (cmd direct-post move).

## Risk notes
RED — truth-ownership + cross-domain SQL projection. Human + design-doc decision.

## Validation
N/A (audit).

## Done criteria
Boundary decision recorded (keep-as-is + rationale, or a move plan) before any code.
