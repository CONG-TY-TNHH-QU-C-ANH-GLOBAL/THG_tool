# Feature: outbound-actions

Tenant-isolated outbound state machine: row-level CAS transitions, additive
transition ledger, policy-driven dedup (`internal/store/outbound/`,
`action_policies`, `execution_attempts`). Supports the
[engagement-approval](../../experiences/engagement-approval/README.md)
experience.

- [technical.md](technical.md) — stable invariants: execution_attempts
  extension, action_policies schema + resolution, transition types, additive
  best-effort ledger writes, row-CAS rationale, tenant-isolation API + linter
  gate, policy-driven dedup contract. Implementation state: **partial**
  (PR1 shipped — dedup.go + seeds; PR2 cleanup staged).
- [implementation/refactor-plan.md](implementation/refactor-plan.md) — the
  staged PR1/PR2 plan, risks, acceptance criteria, resolved decisions,
  non-goals.
- [implementation/append-only-ledger.md](implementation/append-only-ledger.md)
  — design-only event-sourced action_ledger migration (PR A additive column
  shipped; writers not implemented; 2 UPDATE violations tracked by
  check_topology.sh remain — baseline lowered from 3 by ARCHST-R1).
