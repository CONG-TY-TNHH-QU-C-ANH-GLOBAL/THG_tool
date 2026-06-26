---
doc_type: architecture
status: active
owner: platform
last_reviewed: 2026-06-26
related_pr_or_issue: chore/docs-arch-epic-docs1-indexes
---

# docs/architecture/ — Index

Map of architecture standards, boundaries, ownership, and decision records. See
[`../DOCS_GOVERNANCE.md`](../DOCS_GOVERNANCE.md) for the rules and
[`../INDEX.md`](../INDEX.md) for the top-level docs map.

## What belongs here

- Architecture **standards** and the target shape of the system.
- **Module boundaries**, ports/adapters, database ownership.
- Architecture **decision records** (ADRs) under `decisions/`.
- Refactor roadmaps and current-state audits/inventories.

## What does NOT belong here

- Per-feature implementation specs / behavior contracts → root `specs/`.
- Runbooks / validation / deployment notes → `docs/engineering/`.
- Agent workflow / queue → [`../ai/`](../ai/INDEX.md).
- Component-structure rules + hotspots inventory → `specs/platform/`
  (`COMPONENT_STRUCTURE_RULES.md`, `COMPONENT_HOTSPOTS.md`) — those are the
  binding source of truth; link to them, don't duplicate.

## Source-of-truth rules

- The **standard** is [ARCHITECTURE_STANDARD.md](ARCHITECTURE_STANDARD.md); other
  docs here elaborate or record decisions against it.
- Runtime topology + boundary enforcement live in `specs/RUNTIME_TOPOLOGY.md` +
  `scripts/check_topology.sh` (executable invariants) — the authoritative source;
  docs here must not contradict them.
- One topic = one doc. If a doc is replaced, mark it `status: superseded`.

## Contents

| Doc | Purpose |
|---|---|
| [ARCHITECTURE_STANDARD.md](ARCHITECTURE_STANDARD.md) | The official architecture standard (modular monolith / hexagonal / outbox). |
| [MODULE_BOUNDARIES.md](MODULE_BOUNDARIES.md) | Allowed import boundaries between modules. |
| [PORTS_AND_ADAPTERS.md](PORTS_AND_ADAPTERS.md) | Ports/adapters conventions. |
| [DATABASE_OWNERSHIP.md](DATABASE_OWNERSHIP.md) | Per-domain DB/table ownership. |
| [CONNECTOR_STATE_MACHINE.md](CONNECTOR_STATE_MACHINE.md) | Connector lifecycle state machine. |
| [TRANSACTIONAL_OUTBOX.md](TRANSACTIONAL_OUTBOX.md) | Transactional-outbox pattern. |
| [REFACTOR_ROADMAP.md](REFACTOR_ROADMAP.md) | Sequenced refactor roadmap. |
| [CURRENT_CODE_AUDIT.md](CURRENT_CODE_AUDIT.md) | Current-state code audit. |
| [CURRENT_PACKAGE_INVENTORY.md](CURRENT_PACKAGE_INVENTORY.md) | Package inventory snapshot. |
| [DIAGRAM_RECONCILIATION.md](DIAGRAM_RECONCILIATION.md) | Diagram-vs-reality reconciliation. |
| [SONAR_FACTORY_PROTOCOL.md](SONAR_FACTORY_PROTOCOL.md) | Sonar cleanup protocol. |
| [ADR-PR9-DATA-PLATFORM.md](ADR-PR9-DATA-PLATFORM.md) | ADR: data-platform decision (PR9). |

> ADRs: new architecture decision records go under `docs/architecture/decisions/`
> (created on first use). `ADR-PR9-DATA-PLATFORM.md` predates that folder; move it
> there only as part of an audited docs-move item, not ad hoc.

## Where to add a new doc

- A new **standard / boundary / ownership** doc → here, with governance
  frontmatter; add a row to the Contents table.
- A new **decision** → `docs/architecture/decisions/ADR-<id>-<slug>.md`.

## How to archive / update

- Mark superseded docs `status: superseded` (keep for history) and update the
  Contents row; do not silently delete.
