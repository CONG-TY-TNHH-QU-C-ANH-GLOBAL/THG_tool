# Feature: knowledge-os

Org-scoped business knowledge backend (`internal/workspace_knowledge`):
asset ingestion + sanitization, hybrid retrieval with RRF and governance
layers, grounded context assembly, cost accounting, immutable feedback.

- [technical.md](technical.md) — the KnowledgeOS architecture contract.
  Implementation state: **backed** (core foundation shipped; some L5–L7
  layers and vector-DB port-swap deferred per the doc).
- [implementation/postgres-compat.md](implementation/postgres-compat.md) —
  dual-target SQLite/Postgres plan; PR-1/2/3 shipped for this domain.
- [implementation/production-hardening.md](implementation/production-hardening.md)
  — 10-goal hardening dashboard; 7 shipped, 3 designed (partial).
- [runbooks/retrieval-soak.md](runbooks/retrieval-soak.md) — operator soak
  runbook. Its generated report is written to the gitignored
  `artifacts/retrieval-soak/RETRIEVAL_SOAK_REPORT.md` (DOCS-R2: generated
  output is not a tracked spec).
