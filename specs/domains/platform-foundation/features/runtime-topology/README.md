# Feature: runtime-topology

Current runtime composition of the system as it actually runs in production:
process layout, store-domain layering, canonical writers, append-only
boundaries, verified-state reads. Script-enforced — `scripts/check_topology.sh`
runs in CI and fails on new violations.

- [technical.md](technical.md) — the runtime topology contract (§6 is the
  enforced invariant list). Implementation state: **backed** (living doc,
  grep-gated in CI). This is the current runtime authority named by
  `CLAUDE.md`/`AGENTS.md`.
- [evidence/production-flow.md](evidence/production-flow.md) — older living
  production-pipe reference (stale; superseded in practice by technical.md,
  kept pending refresh).
