---
doc_type: engineering
status: active
owner: platform
last_reviewed: 2026-06-29
related_pr_or_issue: chore/docs-y1-relocate-pr-checklist
---

# docs/engineering/ — Index

Engineering runbooks, validation/testing notes, and deployment/release procedures.
See [`../DOCS_GOVERNANCE.md`](../DOCS_GOVERNANCE.md) for the rules and
[`../INDEX.md`](../INDEX.md) for the top-level docs map.

## What belongs here

- Runbooks and operational procedures.
- Validation / testing / release checklists.
- Deployment and environment notes.

## What does NOT belong here

- Architecture standards / boundaries / ADRs → [`../architecture/`](../architecture/INDEX.md).
- Product specs / behavior contracts → root `specs/`.
- Agent workflow / queue → [`../ai/`](../ai/INDEX.md).

## Contents

| Doc | Purpose |
|---|---|
| [PR_CHECKLIST.md](PR_CHECKLIST.md) | Pre-PR checklist mirroring the `CLAUDE.md` Engineering Guardrails (file-size, no god files, DRY/SOLID, tests). |

## Where to add a new doc

A new engineering runbook → here in `docs/engineering/`, with governance frontmatter
(`doc_type: engineering`), and add a row to the Contents table above.
