# Copilot Intent / Routing Architecture

**Status:** ACTIVE_BINDING. **Created:** 2026-06-14. **Track:** Comment Intelligence / Agent-routing.
**Goal:** turn the growing command-parser (`agent_action_router.go`) into a clear pipeline so future typo-tolerant / multilingual NLU has a clean home — without changing behavior yet.

Pipeline:
```
raw user text → normalize → extract entities → classify intent → route to action → workflow executes → outbound queues/audits
```

## Layers

| Layer | Owns | Where (this PR) | Boundary |
|-------|------|-----------------|----------|
| **Normalize** | accent folding, dashboard-context stripping, folded `contains` | `internal/ai/intent_normalize.go` | pure strings; no IO |
| **Lexicon** | named keyword/scope/verb sets | `internal/ai/intent_lexicon.go` | pure data |
| **Entities** | FB URL extraction, scope/crawl-verb/count features, `IntentEntities` | `internal/ai/intent_entities.go` | delegates FB-URL trust to `internal/fburl` |
| **Types** | `Confidence`, `IntentEntities`, `IntentDecision`, `RouteDecision` | `internal/ai/intent_types.go` | pure data |
| **Router** | `deterministicFacebookAction` (intent → action name), `RouteDecisionFor` (observability) | `internal/ai/intent_router.go` | no DB/outbound/session |
| **URL** | host-anchored FB URL recognition + canonicalization | `internal/fburl` | single source of truth |
| **Workflow** | `commentSinglePost`, `queueLeadOutreach`, readiness/coverage/quality/dedup/outbound gates | `cmd/scraper/*` (unchanged) | owns every gate |

All intent files import **only stdlib + `internal/fburl`** (verified `go list`/grep) — no store/outbound/server/connector.

## Phase 1 — Inventory (current routing surface)

**`deterministicFacebookAction` (the classifier)** consumes: `stripDashboardContext`, `foldVietnameseForMatch`, `containsAnyFolded` (normalize); `firstFacebookURL`, `isLikelyFacebookPostURL`, `extractMaxItemsFromPrompt`, `extractIntentEntities`, `promptKeywords` (entities); the lexicon sets.

**Action names emitted:** `inbox_all_leads`, `comment_single_post`, `comment_all_leads`, `create_job_post`, `scrape_comments`, `scrape_group`, `search_groups`, and `""` (no match → brain planner). NOTE: it is one of several action sources (deterministic fast-path, brain planner, LLM fallback). `auto_comment`/`auto_inbox`/`care_fanpage`/`post_to_profile` come from skills/brain, NOT this function.

**Slash commands / skills:** registered in `cmd/scraper/skills_register.go` via `skillThroughHandler(actionID,…)` — the skill ID maps directly to an action handler. This is a **separate path** from `deterministicFacebookAction` and is **untouched** by this PR.

**FB URL helpers:** `firstFacebookURL`/`isLikelyFacebookPostURL` (intent_entities) delegate to `fburl.ExtractFacebookURLs`/`LooksLikePostURL`; gates (`promptIsSelfSufficient`, `promptIsLeadActionSelfSufficient`) also use them. `internal/fburl` imports nothing from `internal/ai` — no cycle.

**Tests protecting routing:** `agent_router_single_post_test.go` (single_post, lookalike, crawl-verb), `intent_router_test.go` (NEW — 10-case characterization + observability), `agent_self_sufficient_test.go` (self-sufficiency gates), `routing_decision_test.go` (signal analysis), `agent_policy_test.go` (org auto-policy), `agent_brain_test.go` (brain plan validation).

## Decision — in-package split now, leaf package proposed next

A true leaf package `internal/ai/intent` was evaluated and **deferred** because the normalization helpers (`foldVietnameseForMatch`, `stripDashboardContext`) and `inferCrawlTargetingFromPrompt` are **shared across 5–6 `ai` files** (agent, agent_preflight, agent_request, agent_responses, agent_brain, routing_decision) and tangled with business-context inference. Extracting them would force a 6-file import ripple and risks a cycle (the routing gates `agent.go` calls would need `ai` symbols back). Per the spec's escape hatch and the repo's staged-evolution rule, this PR does the **behavior-preserving in-package split** (pipeline files in `package ai`, zero ripple, zero cycle risk).

**Proposed next (separate move-only PR):** promote `internal/ai/intent` to a real package once the normalization helpers are consolidated and the gate call-graph is decoupled — making the no-store/outbound boundary compile-enforced. Until then the boundary is grep-verifiable (intent_*.go import only fburl + stdlib).

## What moved vs stayed (this PR)

- **Moved into `intent_*.go`** (same package, behavior-identical): `deterministicFacebookAction`, `containsAnyFolded`, `foldVietnameseForMatch` (from agent_preflight), `stripDashboardContext` (from agent_request), `firstFacebookURL`, `isLikelyFacebookPostURL`, `extractMaxItemsFromPrompt`, `promptKeywords`; keyword literals → named lexicon vars; + new `IntentEntities`/`IntentDecision`/`RouteDecision` types and `extractIntentEntities`/`RouteDecisionFor`.
- **Stayed:** the self-sufficiency gates + `inferredTargetingSummary` + arg utils (agent_action_router.go, now 185 lines, down from 372); `inferCrawlTargetingFromPrompt` (agent_preflight.go); all workflow/outbound gates (`cmd/scraper`).
- **Behavior:** preserved — the classifier branches are byte-equivalent (named lexicons = the same literals; entity flags = the same inline expressions). Locked by the existing + new characterization tests.

## Future NLU home

Typo-tolerant / multilingual matching adds variants to `intent_lexicon.go` and/or a fuzzy matcher behind `intent_normalize.go`/`intent_entities.go`, and richer `Confidence` in the classifier — without touching the workflow/outbound gates. TODO cases are stubbed in `intent_router_test.go` (not failing).
