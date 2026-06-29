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
boundary_target: leaf-move
---

# ARCHCP3 — Extract copilot/intent subpackage

## DIRECTION (2026-06-29): Option B, incrementally — phase 1 shipped
Self-selected **Option B** (core intent move + separate generic `textnorm` leaf) under
Autonomy v2, confirmed by a senior-architect feasibility pass — NOT Option A's ~60-edit
big-bang (which would publish generic helpers as `intent.*` and pollute the boundary).
Reached incrementally so the E3 boundary question is resolved by construction:
- **Phase 1 — DONE in [`ARCHCP3a`](ARCHCP3a-textnorm-leaf.md)** (branch
  `chore/archcp3-textnorm-leaf`): extracted the generic `Fold`/`ContainsAny` helpers to
  the neutral leaf `internal/drivers/copilot/textnorm`, wrapper-first (shims keep all 42
  call sites unchanged). This removes the heaviest, non-intent refs from the eventual move.
- **Phase 1b — DONE in [`ARCHCP3b`](ARCHCP3b-classifier-complexity.md)** (branch
  `chore/archcp3b-classifier-complexity`): reduced `deterministicFacebookAction` (28 →
  ≤15) via verbatim per-branch helper extraction. The bulk textnorm-call migration was
  attempted and abandoned (it made the 28-complexity classifier New Code + crossed a
  near-200 caller); this reduction is the real unblocker so any later phase can touch
  the classifier. The textnorm shims persist (no caller churn).
- **Phase 2 (optional / deferred):** rewrite the 42 call sites to `textnorm.*`; drop the
  shims. Not required before phase 3 — the shims can stay in copilot.
- **Phase 3:** move the genuinely-intent files into `internal/drivers/copilot/intent/`
  (~6 real intent exports, ~10 remaining refs) — the original ARCHCP3 deliverable, now
  small and boundary-clean. Stays open until phases 2–3 land.

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
