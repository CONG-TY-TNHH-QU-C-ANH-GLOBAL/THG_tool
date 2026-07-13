# Feature: account-safety

Shared safety feature: per-account automation-runtime state machine, machine
and per-account concurrency budgets, risk budgets, cooldown, and operator
visibility — **coordination via safety budgets, not evasion**. Consumed by
[multi-group-fresh-lead-crawl](../multi-group-fresh-lead-crawl/README.md) and
any future automation workflow on the
[fresh-lead-discovery](../../experiences/fresh-lead-discovery/README.md)
experience.

- [technical.md](technical.md) — the technical contract (PR-C0.5): Account
  Safety Coordinator, account-runtime state machine (with operator messages),
  concurrency policy, risk budgets, telemetry contract.
- [decisions/safety-boundaries.md](decisions/safety-boundaries.md) — binding
  hard boundaries (no evasion/solving/rotation) and data-plane ownership.
- [runbooks/review-checklist.md](runbooks/review-checklist.md) — the checklist
  applied to every PR-C* runtime PR.

Implementation state: **partial** — `internal/session/accountsafety/` (pure
policy + tests) and the `session.Allocator` lease exist; the Coordinator wiring
(PR-C3) is not yet built.
