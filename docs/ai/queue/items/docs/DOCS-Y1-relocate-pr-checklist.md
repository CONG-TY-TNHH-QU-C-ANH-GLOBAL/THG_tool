---
id: DOCS-Y1
status: REVIEW
lane: YELLOW
risk: YELLOW
depends_on: [DOCS1]
parallel_safe: false
branch: "chore/docs-y1-relocate-pr-checklist"
pr_url: ""
---

# DOCS-Y1 — Relocate docs/PR_CHECKLIST.md into docs/engineering/

## Goal
docs/PR_CHECKLIST.md sits loose in docs/ root; it is an engineering runbook and belongs under docs/engineering/. Move it and update the one referrer.

## Doc area
docs/ root -> docs/engineering/.

## Files likely involved
- git mv docs/PR_CHECKLIST.md docs/engineering/PR_CHECKLIST.md
- EDIT AGENTS.md (line referencing `docs/PR_CHECKLIST.md`)
- create docs/engineering/INDEX.md (first doc in the category)

## Dependencies
DOCS1 (taxonomy/index conventions established first).

## Risk notes
YELLOW — the only referrer is AGENTS.md (a root entrypoint); NO script references it (audited: grep found only AGENTS.md). Use `git mv` to preserve history; update the AGENTS.md link in the SAME PR. Verify no other doc links to it before moving.

## Validation
grep -rIn "PR_CHECKLIST" . (only the updated AGENTS.md ref remains) ; bash scripts/check_docs_governance.sh ; bash scripts/ai_validate.sh

## Done criteria
PR_CHECKLIST.md under docs/engineering/ with history preserved; AGENTS.md reference updated; docs/engineering/INDEX.md created; no dangling references.
