# Feature: multi-group-fresh-lead-crawl

Campaign/queue/run orchestration that crawls many Facebook groups with an
account pool and mints **fresh leads only** (server-defined cutoff, provable
timestamps, temporal-frontier early stop). Supports the
[fresh-lead-discovery](../../experiences/fresh-lead-discovery/README.md)
experience. Layers on [account-safety](../account-safety/README.md); never
weakens it.

- [technical.md](technical.md) — the technical contract (PR-M0): orchestration
  model, freshness gate, timestamp-confidence model, temporal frontier, dedupe,
  data-model invariants, failure semantics, acceptance criteria.
- [implementation/postgres-schema.md](implementation/postgres-schema.md) — the
  shipped PostgreSQL platform schema (PR-M2B, migrations 0112–0117). The
  authoritative DDL.
- [implementation/code-organization.md](implementation/code-organization.md) —
  runtime code organization blueprint (PR-M3..M5): package shape, ports,
  transaction seams.
- [implementation/rollout.md](implementation/rollout.md) — PR train M0–M6 and
  rollback.
- [decisions/rejected-designs.md](decisions/rejected-designs.md) — binding
  rejected alternatives.
- [evidence/crawl-speed-checkpoint-audit.md](evidence/crawl-speed-checkpoint-audit.md)
  — the PR-C0 audit that grounds this feature (supporting evidence, not a
  decision record).

Implementation state: **partial** — schema (0112–0117) and store mechanics
(`internal/store/crawlrun/`, `internal/services/facebook/crawlcampaign/`) are
merged and dormant; the PR-M4 scheduler wiring is not yet built.
