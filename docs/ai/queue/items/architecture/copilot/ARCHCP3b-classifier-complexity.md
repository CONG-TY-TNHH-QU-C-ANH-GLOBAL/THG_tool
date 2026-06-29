---
id: ARCHCP3b
status: DONE
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCP3a]
parallel_safe: false
branch: "chore/archcp3b-classifier-complexity"
pr_url: https://github.com/CONG-TY-TNHH-QU-C-ANH-GLOBAL/THG_tool/pull/169
boundary_target: prep-extraction
---

# ARCHCP3b — Reduce deterministicFacebookAction complexity (unblocks ARCHCP3 phases 2/3)

## Why (re-scoped from the bulk textnorm-call migration)
The planned phase-2 (rewrite the 42 `textnorm` call sites + drop the ARCHCP3a shims)
was attempted and **abandoned**: it tripped two file-level guards because it edits the
hot copilot caller files — `go_cognitive_check` flagged `deterministicFacebookAction`
(complexity **28**, with 13 `textnorm` calls inside it → New Code), and `check_file_size`
flagged `agent_business_context.go` (201 after one import line). That churn is exactly
what the ARCHCP3a wrapper-first shims were built to avoid. Senior/Ponytail call: the
bulk rewrite is the wrong slice; the **real blocker** is the 28-complexity classifier
that EVERY ARCHCP3 phase must touch.

## What this slice does
Decompose `deterministicFacebookAction` (intent_router.go, 28) into a flat dispatch +
six single-rule `classifyX` helpers (`classifyInboxBulk`, `classifyCommentSingle`,
`classifyCommentBulk`, `classifyPostingAction`, `classifyScrape`, `classifySearch`).
Verbatim branch logic, exact ladder order, exact args mutations + early returns. Main +
all helpers ≤15. intent_router.go 142 → 189 (no new file; under 200). No import change,
no new package, no RED zone (pure classification). The ARCHCP3a textnorm shims stay
untouched (no caller churn).

## Behavior preservation
Pinned by the existing comprehensive characterization in `intent_router_test.go`: all
7 routes (inbox_all_leads / comment_single_post / comment_all_leads / create_job_post /
scrape_comments / scrape_group / search_groups) + no-match + multi-URL + ask-for-link,
plus the `RouteDecisionFor` branch-flag tests — all pass unchanged.

## Rollback
Revert the commit: the six helpers re-inline into the single function. Pure refactor.

## Effect on ARCHCP3
Removes the primary New-Code complexity landmine on the copilot intent track. Phase 3
(move the intent files into `internal/drivers/copilot/intent/`) can now touch the
classifier without a forced 28→15 reduction mid-move. The optional textnorm-call
migration (old phase 2) is deferred — the shims are fine to persist in copilot.

## Validation
go build/vet/test ./... green; check_topology + go_cognitive_check (≤15) + check_file_size
+ import-boundary (no new violation) + ai_validate pass. On merge → DONE.
