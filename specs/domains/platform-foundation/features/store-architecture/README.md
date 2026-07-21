# Feature: store-architecture

The `internal/store/` subpackage decomposition: bounded store domains
(leads, threads, outbound, coordination, crawl, knowledge, connectors,
identities, app, prompts) with locked dependency direction and truth
ownership. Pairs with [runtime-topology](../runtime-topology/README.md),
which enforces the boundaries in CI; the spatial map lives in
`internal/store/DOMAINS.md`.

- [technical.md](technical.md) — the canonical subpackage decomposition
  design (locks L1–L4). Implementation state: **backed** (the subpackages it
  specifies exist).
- [evidence/phase-5a-coordination-audit.md](evidence/phase-5a-coordination-audit.md)
  — historical one-time extraction-readiness audit that gated Phase 5B
  (archived; the extraction has since shipped).
