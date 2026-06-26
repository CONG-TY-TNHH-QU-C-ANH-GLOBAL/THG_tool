---
doc_type: engineering
status: active
owner: platform
last_reviewed: 2026-06-26
related_pr_or_issue: chore/agentic-workflow-setup
---

# Documentation Governance

Keeps Markdown, specs, plans, and debt docs from sprawling at the repo root.
Read this before creating or moving any doc. Enforced (warn-only for legacy
root docs) by `scripts/check_docs_governance.sh`.

## Root markdown policy

- `README.md` allowed.
- `AGENTS.md` allowed as the cross-agent entrypoint.
- `CLAUDE.md` allowed as the Claude Code entrypoint.
- `SPEC_GOVERNANCE.md` allowed (spec governance entrypoint).
- Existing legacy root docs may remain temporarily if listed in the script allowlist.
- New project docs / specs / plans / debt docs must NOT be added to root.

## Doc categories

- `docs/business/` — product, customer, market, operator workflow, requirements
- `docs/architecture/` — architecture standards, boundaries, ADRs, module ownership
- `docs/architecture/decisions/` — architecture decision records (ADRs)
- `docs/specs/` — implementation specs (active / accepted / archived)
- `docs/engineering/` — runbooks, validation, testing, deployment notes
- `docs/debt/` — technical debt, Sonar debt, cleanup plans, risk registers
- `docs/ai/` — agentic workflow, autopilot queue, escalation playbook, agent reports

## Required metadata for new governance-controlled docs

- `doc_type`
- `status`
- `owner`
- `last_reviewed`
- `related_pr_or_issue`

Suggested frontmatter template:

```markdown
---
doc_type: architecture | business | spec | engineering | debt | ai
status: draft | active | accepted | superseded | archived
owner: team-or-person
last_reviewed: YYYY-MM-DD
related_pr_or_issue: PR-or-issue-link-or-none
---
```

## Rules

- No duplicate source-of-truth docs.
- If replacing a doc, mark the old one `status: superseded` or move it to an archive folder.
- Technical debt docs go under `docs/debt/`, not random root files.
- Business / product docs go under `docs/business/`.
- Architecture decisions go under `docs/architecture/decisions/`.
- Agent workflow docs go under `docs/ai/`.

## Migration note

This governance is additive. Do NOT bulk-move existing root/spec/debt docs in the
same PR that introduces governance — that is a separate queue item
("Docs taxonomy migration"). Preserve history with `git mv` and update references.
