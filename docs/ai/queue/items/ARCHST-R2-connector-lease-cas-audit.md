---
id: ARCHST-R2
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHST-R2 — AUDIT: connector pairing lease / CAS consistency

## Goal (audit-only)
Determine whether the comment-reverify claim lease and the connector pairing claim/lease are independent or should be unified; document divergence if intentional.

## Component / domain
store/connectors CAS/lease + coordination reverify queue. RED.

## Files likely involved
connectors/connector_pairing.go, coordination/comment_reverify*.go.

## Dependencies
None.

## Risk notes
RED — connector CAS/lease/ownership semantics. Human decision; no autonomous change.

## Validation
N/A (audit).

## Done criteria
Decision record: unify or document-divergence, with rationale for future maintainers.
