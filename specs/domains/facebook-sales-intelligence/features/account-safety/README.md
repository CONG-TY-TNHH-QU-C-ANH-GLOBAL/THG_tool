# Feature: account-safety

Shared safety feature: per-account automation-runtime state machine, machine
and per-account concurrency budgets, risk budgets, cooldown, and operator
visibility — **coordination via safety budgets, not evasion**. Consumed by
[multi-group-fresh-lead-crawl](../multi-group-fresh-lead-crawl/README.md) and
any future automation workflow on the
[fresh-lead-discovery](../../experiences/fresh-lead-discovery/README.md)
experience.

- [technical.md](technical.md) — the technical contract (PR-C0.5) and owner of
  every binding invariant: Account Safety Coordinator, hard boundaries (no
  evasion/solving/rotation), account-runtime state machine (with operator
  messages), concurrency policy, risk budgets, telemetry contract, data-plane
  ownership.
- [decisions/ADR-001-conservative-account-safety.md](decisions/ADR-001-conservative-account-safety.md)
  — supporting rationale: alternatives considered, throughput-vs-safety
  trade-offs, consequences.
- [runbooks/review-checklist.md](runbooks/review-checklist.md) — the checklist
  applied to every PR-C* runtime PR.
- [implementation/reliability-track.md](implementation/reliability-track.md) —
  founder-directed reliability PR train (verified initiator/account/connector/
  identity/eligibility before any action; no silent fallback). PR-A shipped;
  its scope spans crawl/comment/inbox actions.

Implementation state: **partial** — `internal/session/accountsafety/` (pure
policy + tests) and the `session.Allocator` lease exist; the Coordinator wiring
(PR-C3) is not yet built.
