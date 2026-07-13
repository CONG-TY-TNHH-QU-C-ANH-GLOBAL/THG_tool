# Feature: lead-ingestion

Deterministic, characterization-pinned ingestion of crawled posts into leads
(`internal/leadingest`). Supports the
[fresh-lead-discovery](../../experiences/fresh-lead-discovery/README.md) and
[lead-management](../../experiences/lead-management/README.md) experiences.

- [technical.md](technical.md) — the test-pinned behavior contract for
  `leadingest.IngestPost` (filtering, routing, gating, persistence,
  notification hooks). Implementation state: **backed** (characterization
  tests in `internal/leadingest/`).
