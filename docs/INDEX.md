---
doc_type: engineering
status: active
owner: platform
last_reviewed: 2026-06-26
related_pr_or_issue: chore/agentic-workflow-setup
---

# Docs Index

Where to find documentation. See `docs/DOCS_GOVERNANCE.md` for the rules on
adding or moving docs. Categories are created on first use — an empty/absent
folder just means nothing has landed there yet.

## Business
`docs/business/` — product, customer, market, operator workflow, requirements.

## Architecture
`docs/architecture/` — architecture standards, module ownership, boundaries.
See the subtree map: [`docs/architecture/INDEX.md`](architecture/INDEX.md).
`docs/architecture/decisions/` — architecture decision records (ADRs).

## Specs
`docs/specs/` — implementation specs (active / accepted / archived).
Authoritative specs currently live under the root `specs/` tree — see
[`specs/README.md`](../specs/README.md) (overview) and
[`specs/SPEC_REGISTRY.json`](../specs/SPEC_REGISTRY.json) (the machine-readable
source of truth for what specs exist and how trusted they are). Reconciling the
root `specs/` home with this governed taxonomy is tracked as queue item DOCS-R1
(audit-only).

## Engineering
`docs/engineering/` — runbooks, validation, testing, deployment notes.
- `docs/DOCS_GOVERNANCE.md` — documentation governance.

## Technical Debt
`docs/debt/` — technical debt, Sonar debt, cleanup plans, risk registers.

## AI Workflow
`docs/ai/` — agentic development workflow. Subtree map:
[`docs/ai/INDEX.md`](ai/INDEX.md).
- `docs/ai/AUTOPILOT_QUEUE.md` — next-PR queue.
- `docs/ai/ESCALATION_PLAYBOOK.md` — hard-case protocol.
- `docs/ai/AGENT_REPORT_TEMPLATE.md` — PR report template.
- `docs/ai/queue/INDEX.md` — architecture/docs decomposition queue map.
- `docs/ai/COGNITIVE_COMPLEXITY_GUARD.md` — local S3776 approximation run in `ai_validate.sh`.

## Root Entrypoints
- `AGENTS.md` — cross-agent entrypoint (thin).
- `CLAUDE.md` — Claude Code entrypoint (thin).
- `README.md` — project readme (if present).
