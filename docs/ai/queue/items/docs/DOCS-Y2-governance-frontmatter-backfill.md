---
id: DOCS-Y2
status: READY
lane: YELLOW
risk: YELLOW
depends_on: [DOCS1]
parallel_safe: false
branch: ""
pr_url: ""
---

# DOCS-Y2 — Backfill governance frontmatter across docs/ (staged)

## Goal
Bring docs/ docs lacking the required governance frontmatter (doc_type/status/owner/last_reviewed/related_pr_or_issue) into compliance, in small staged batches per subtree.

## Doc area
docs/ (architecture, ai) — excludes specs/ (separate governance, see DOCS-R1).

## Files likely involved
docs/architecture/*.md, docs/ai/*.md lacking frontmatter; one subtree per PR.

## Dependencies
DOCS1. Overlaps DOCS2 for docs/architecture — sequence: do DOCS2 (architecture) first, then DOCS-Y2 covers docs/ai/ and any remainder.

## Risk notes
YELLOW only because it touches many files — additive frontmatter, no content change. Keep batches small (one subtree per PR) to stay reviewable. No script behavior depends on this frontmatter today (guard checks existence only).

## Validation
bash scripts/check_docs_governance.sh ; bash scripts/ai_validate.sh

## Done criteria
Targeted subtree's docs carry compliant frontmatter; no content/behavior change; batches small and reviewable.
