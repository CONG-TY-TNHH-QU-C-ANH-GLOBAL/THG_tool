---
id: ARCHCP3
status: BLOCKED
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCP1, ARCHCP2]
parallel_safe: false
branch: ""
pr_url: ""
blocked_on: human-boundary-decision
---

# ARCHCP3 — Extract copilot/intent subpackage

## Goal
Move the pure intent-classification cluster (intent_normalize/lexicon/types/entities/router) into a bounded `copilot/intent/` subpackage with a small facade (DeterministicFacebookAction, RouteDecisionFor).

## Component / domain
internal/drivers/copilot intent classification.

## Files likely involved
intent_*.go (+ intent_*_test.go) → internal/drivers/copilot/intent/; agent.go / agent_action_router.go / agent_preflight.go call sites updated to intent.* facade.

## Dependencies
ARCHCP1, ARCHCP2 (reduce in-package complexity before moving boundaries; mirrors the internal/ai staged precedent).

## Risk notes
YELLOW move-only. Verified ZERO cross-cluster cycle: intent_* references no
agent_* symbols; agent→intent is one-way; the normalize helpers are used *within*
the cluster (intent_router/intent_entities), so the whole cluster moves together.

## Blast radius (CORRECTED 2026-06-27 — supersedes the original "~5 call sites")
The original "~5 internal call sites, small facade" estimate was stale (written
before ARCHCP2, which added agent_business_context.go + agent_crawl_targeting.go,
both heavy users of the normalize helpers). Verified actual map:

- **8 functions must be exported** to support external callers:
  RouteDecisionFor (already exported), DeterministicFacebookAction,
  PromptIsDirectPostComment, FirstFacebookURL, ExtractMaxItemsFromPrompt,
  PromptKeywords, FoldVietnameseForMatch, StripDashboardContext, ContainsAnyFolded.
- **~60 call sites across 11 non-intent files** must be rewritten to `intent.*`:
  agent.go, agent_action_router.go, agent_request.go, agent_business_context.go,
  agent_crawl_targeting.go, routing_decision.go, brain_action_prep.go,
  brain_plan_validator.go, agent_responses.go, + agent_direct_comment_p0_test.go,
  agent_router_single_post_test.go.
- The 3 heaviest exports (foldVietnameseForMatch / stripDashboardContext /
  containsAnyFolded, ~34 refs) are **generic text normalization**, NOT intent
  classification — moving them makes them `intent.*` public API.

## BLOCKED — E3 architecture boundary decision (awaiting founder)
This is a single bigger PR (11 files, ~60 mechanical edits) that publishes
generic helpers as `intent` API — it collides with staged-evolution / no-big-bang
and the reviewable-diff rule, so it must not be auto-executed on the stale
estimate. Decide the boundary, then unblock:

- **Option A — full move:** all 5 intent_* files → `copilot/intent/`, export the
  8 functions, rewrite ~60 sites. Mechanical, behavior-preserving, no cycle.
  Accepts generic helpers as `intent.*` API.
- **Option B — core move + textnorm leaf:** also split fold/strip/contains into a
  neutral `copilot/textnorm/` leaf so `intent/` exposes only real intent API.
  Cleaner boundary; adds a 2nd new package (beyond move-only).
- **Option C — defer:** leave the cluster in-package; the in-package complexity
  was already reduced by ARCHCP1/ARCHCP2, so the boundary move is optional.

## Validation
go build ./... ; go vet ./internal/drivers/copilot/... ; go test ./... ; ai_validate.sh

## Done criteria
copilot/intent/ exists with files + doc + tests moved; copilot package imports
./intent via the chosen facade; no cycle; behavior identical; move-only. NOTE: the
"small facade (2 funcs)" criterion is not achievable as written — see Blast radius;
re-state the facade per the chosen option before implementing.
