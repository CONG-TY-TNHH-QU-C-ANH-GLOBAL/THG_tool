# Domain: knowledge-platform

Ownership domain (kind: **product_platform**) — org-scoped business knowledge:
asset ingestion, calibration, governed retrieval, and grounded context
assembly that product domains (Dashboard Chat, Telegram, comment
intelligence) build on. RAG is a separate retrieval plane behind policy and
ACL boundaries; it never blurs into the SQLite/PostgreSQL planes.

Structure: `features/` hold technical contracts and their implementation /
runbooks / evidence.

## Features

- [knowledge-os](features/knowledge-os/README.md) — the KnowledgeOS backend:
  org-scoped assets → retrieval (hybrid + RRF + governance) → context
  assembly, with soak harness, dual-target SQLite/Postgres storage, and the
  production-hardening goal dashboard.
