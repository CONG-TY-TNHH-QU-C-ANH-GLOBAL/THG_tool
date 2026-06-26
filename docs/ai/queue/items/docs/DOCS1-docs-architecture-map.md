---
id: DOCS1
status: DONE
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: true
branch: chore/docs-arch-epic-docs1-indexes
pr_url: https://github.com/CONG-TY-TNHH-QU-C-ANH-GLOBAL/THG_tool/pull/127
---

# DOCS1 — Docs architecture map (subtree indexes + backlink fix)

## Goal
Make docs/ navigable: add per-subtree index maps for the two populated subtrees and fix the stale spec backlink in docs/INDEX.md. Additive only.

## Doc area
docs/ navigation (docs/ai/, docs/architecture/, docs/INDEX.md).

## Files likely involved
- NEW docs/ai/INDEX.md (map of AUTOPILOT_QUEUE / ESCALATION_PLAYBOOK / AGENT_REPORT_TEMPLATE / queue/)
- NEW docs/architecture/INDEX.md (map of the 12 architecture docs)
- EDIT docs/INDEX.md (fix `specs/SPEC_INDEX.md` -> `specs/README.md` + `specs/SPEC_REGISTRY.json`; link the new sub-indexes + docs/ai/queue/INDEX.md)

## Dependencies
None — first executable item of the Docs Architecture Epic.

## Risk notes
GREEN. Pure docs; no file moves, no script/CLAUDE/registry change, no production code. docs/specs/ and docs/engineering/ indexes intentionally NOT created (empty categories — created on first use, per docs/INDEX.md).

## Validation
- bash scripts/check_docs_governance.sh
- bash scripts/ai_queue_check.sh
- bash scripts/ai_validate.sh ; git diff --check

## Done criteria
docs/ai/INDEX.md + docs/architecture/INDEX.md exist (what-belongs / what-doesn't / source-of-truth / where-to-add / how-to-archive); docs/INDEX.md backlink fixed and links the sub-indexes; governance check OK; no orphan introduced; no move.
