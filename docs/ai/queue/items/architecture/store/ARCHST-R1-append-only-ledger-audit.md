---
id: ARCHST-R1
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHST-R1 — AUDIT: append-only ledger UPDATE violations

## Goal (audit-only — DO NOT implement)
Decision record for the documented append-only violations: MarkActionLedgerOutcome* and engagement_reconcile issue UPDATEs against action_ledger, which downstream projections (leads.engagement_state) read.

## Component / domain
store/coordination truth ownership (action_ledger). RED.

## Files likely involved
coordination/action_ledger.go, coordination/engagement_reconcile.go, a new corrections table + migration.

## Dependencies
Drives specs/APPEND_ONLY_LEDGER_MIGRATION.md staged plan.

## Risk notes
RED — schema/migration + truth-ownership + append-only invariant. Human decision required. Produce an Escalation decision record (docs/ai/ESCALATION_PLAYBOOK.md); do not change semantics autonomously.

## Validation
N/A (audit). When approved, the staged PR carries characterization + migration tests.

## Done criteria
Decision record written; corrections-table design + re-derivation approach approved by a human before any code.
