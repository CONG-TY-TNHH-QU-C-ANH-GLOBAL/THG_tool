---
id: DOCS2
status: READY
lane: GREEN
risk: GREEN
depends_on: [DOCS1]
parallel_safe: true
branch: ""
pr_url: ""
---

# DOCS2 — Backlinks + governance frontmatter audit for docs/architecture

## Goal
Each docs/architecture/* doc gets a one-line backlink to docs/architecture/INDEX.md and (where missing) the governance frontmatter (doc_type/status/owner/last_reviewed/related_pr_or_issue). Additive metadata only.

## Doc area
docs/architecture/.

## Files likely involved
docs/architecture/*.md (12 files) — header backlink + frontmatter only.

## Dependencies
DOCS1 (the architecture INDEX must exist to link to).

## Risk notes
GREEN — additive headers, no content rewrite, no move, no script change. Keep diffs to frontmatter + one backlink line per file.

## Validation
bash scripts/check_docs_governance.sh ; bash scripts/ai_validate.sh

## Done criteria
Every docs/architecture/*.md has frontmatter + an INDEX backlink; no content/behavior change.
