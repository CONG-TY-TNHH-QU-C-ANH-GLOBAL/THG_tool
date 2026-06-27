---
doc_type: ai
status: active
owner: platform
last_reviewed: 2026-06-26
related_pr_or_issue: chore/docs-arch-epic-docs1-indexes
---

# docs/ai/ — Index

Map of the agentic-development workflow docs. See
[`../DOCS_GOVERNANCE.md`](../DOCS_GOVERNANCE.md) for the rules and
[`../INDEX.md`](../INDEX.md) for the top-level docs map.

## What belongs here

- Agentic workflow operating docs: the autopilot queue, the escalation
  protocol, and the agent report template.
- The architecture/decomposition **queue** (items + their human map).

## What does NOT belong here

- Product specs or behavior contracts → root `specs/` (see
  [`../../specs/README.md`](../../specs/README.md)).
- Architecture standards / ADRs → [`../architecture/`](../architecture/INDEX.md).
- Engineering runbooks → `docs/engineering/`.
- Source code or generated test artifacts.

## Source-of-truth rules

- **Queue policy + lifecycle:** [`AUTOPILOT_QUEUE.md`](AUTOPILOT_QUEUE.md) is the
  stable index/policy. It must NOT be used as a mutable status board.
- **Per-item state:** each `queue/items/<domain>/<ID>.md` frontmatter is the
  source of truth for that item's status/lane/deps. Items are grouped by stable
  domain/component (never by status); the tooling discovers them recursively, so
  the folder never affects `depends_on`. The queue map below is only a view.
- **Hard-case protocol:** [`ESCALATION_PLAYBOOK.md`](ESCALATION_PLAYBOOK.md)
  (decision-record format for RED/ambiguous cases).

## Contents

| Doc | Purpose |
|---|---|
| [AUTOPILOT_QUEUE.md](AUTOPILOT_QUEUE.md) | Stable queue index + operating policy (lanes, lifecycle, sprint mode). |
| [ARCHITECT_SPRINT_MODE.md](ARCHITECT_SPRINT_MODE.md) | Senior-architect mission mode (`/thg-architect`): higher-leverage slices, combined GREEN batches, throughput/autonomy/PR-size rules over the existing safety guards. |
| [ESCALATION_PLAYBOOK.md](ESCALATION_PLAYBOOK.md) | Hard-case protocol + decision-record format. |
| [AGENT_REPORT_TEMPLATE.md](AGENT_REPORT_TEMPLATE.md) | Required completion-report shape. |
| [COGNITIVE_COMPLEXITY_GUARD.md](COGNITIVE_COMPLEXITY_GUARD.md) | Local S3776 approximation (`scripts/go_cognitive_check.sh`) — fails on >15 cognitive complexity in changed Go files. |
| [queue/INDEX.md](queue/INDEX.md) | Human map of all queue items (grouped by component + lane, with dependency chains). |
| [queue/items/](queue/items/) | One file per queue item — the source of truth. |

## Where to add a new doc

- A new **queue item** → `queue/items/<domain>/<ID>-<slug>.md` under the owning
  domain folder (`architecture/<component>/` or `docs/`); follow an existing
  item's frontmatter and register it in [`queue/INDEX.md`](queue/INDEX.md).
- A new **workflow/protocol** doc → here in `docs/ai/`, with governance
  frontmatter, and add a row to the Contents table above.

## How to archive / update

- Set `status: superseded` (or `archived`) in the doc's frontmatter rather than
  deleting; update the referring index row.
- Never delete a doc listed in `scripts/check_docs_governance.sh`'s
  `required_docs` (AUTOPILOT_QUEUE / ESCALATION_PLAYBOOK / AGENT_REPORT_TEMPLATE) —
  removing one fails the guard.
