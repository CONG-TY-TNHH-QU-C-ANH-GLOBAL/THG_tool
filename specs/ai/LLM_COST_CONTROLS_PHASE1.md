# LLM Cost Controls — Phase 1

**Branch:** `fix/llm-cost-controls-phase1` (base `origin/main` `3c026588`). Audit source:
`origin/audit/llm-cost-governance` (`f931cc34`, `specs/LLM_COST_GOVERNANCE_AUDIT.md`).
Not merged, not deployed.

Implements the first practical cost-control layer for the **classifier** (`UniversalClassify` — the
highest-volume burner: one call per crawled post). Models, comment-generation behaviour, and all safety
guards are unchanged.

## Implemented now

1. **Structured JSON usage logs** (`internal/ai/classifier_cache.go:logClassifierUsage`). One JSON line
   per classifier decision (`live_call` success/failure, `cache_hit`, `cache_miss`). Safe fields only:
   `event=llm_usage, task_type=classifier, model, success, error_code, latency_ms, prompt_tokens,
   completion_tokens, total_tokens, tokens_unknown, retry_count, cache_enabled, cache_hit, cache_reason,
   cache_key_hash_prefix, reason`. Parseable by ELK/Datadog/CloudWatch (it is plain JSON).
   **org_id / user_id / workflow_id are NOT available at the `UniversalClassify` call site** (it receives
   only `(postContent, authorName, profile, intent)`), so they are intentionally omitted rather than
   invented — see follow-up to thread them.
2. **Exact classifier result cache** (`classifierCache`): process-local, bounded, TTL'd, `map`+`sync.Mutex`,
   opportunistic expiry on read + bounded eviction on write, **no background goroutine**, value-type
   entries (Get returns a copy). Caches only validated successful results; never errors/invalid/refusals.
3. **Real token capture**: `callOpenAIStrictJSON` (single caller — `UniversalClassify`) now returns
   OpenAI `usage`, so live-call logs carry real prompt/completion/total tokens. Comment generation
   (`callOpenAI`) is untouched.
4. **Config knobs** (env, opt-out): `LLM_CLASSIFIER_CACHE_ENABLED` (default `true`),
   `LLM_CLASSIFIER_CACHE_TTL_SECONDS` (default `21600` = 6h), `LLM_CLASSIFIER_CACHE_MAX_ENTRIES`
   (default `5000`). Rollback = set `LLM_CLASSIFIER_CACHE_ENABLED=false`.

**Memory bound:** ≤ `MAX_ENTRIES` entries; each ≈ 64-char key + a small `UniversalClassifyResult`
(4 short strings) ≈ ~0.5–1 KB → **~2.5–5 MB at 5000 entries**. Hard-bounded; safe for the current VPS.

**Cache key** (`classifierCacheKey`): full **SHA-256** of `"clf-v1" ⧉ model ⧉ exact_composed_prompt ⧉
schema_json`. The composed prompt already embeds the business-profile block, the per-crawl intent block,
the author, the language rule, AND the fixed instruction template — so model/prompt/profile/intent/schema
drift all invalidate automatically, and orgs cannot cross-contaminate (different profile → different key).
Hashing the exact composed prompt is **strictly stronger** than enumerating fields (no influencing input
can be forgotten) and is **exact-match** (no normalization → no cache poisoning). Logs expose only a
12-char digest prefix; never raw text.

**Concurrency:** the mutex protects map integrity. Two goroutines that miss the **same** key concurrently
will both call OpenAI (no singleflight) — accepted Phase-1 limitation (rare for distinct crawled posts).

**Multi-pod limitation:** process-local; a hit on one process is a miss on another. Accepted for Phase 1.

## Self-challenge (Step 10 classification)

| Idea | Decision | Why |
|---|---|---|
| Exact classifier cache | IMPLEMENTED_NOW | small, tested, no schema, config-rollback |
| Structured usage JSON logs | IMPLEMENTED_NOW | additive, safe fields only |
| Capture real tokens via single-caller strictJSON | IMPLEMENTED_NOW | single caller, no comment-path impact |
| Conservative post normalization for higher hit-rate | PROPOSE_ONLY | exact-match avoids poisoning; measure hit-rate first |
| singleflight de-dup of concurrent same-key misses | PROPOSE_ONLY | panic-surface + dep promotion; low marginal value now → `fix/classifier-cache-singleflight` |
| Thread org_id/user_id/workflow_id into classifier logs | PROPOSE_ONLY | needs signature plumbing through ingest → `fix/llm-model-routing-and-budget-guards` |
| Embedding `usage.total_tokens` → `RecordEmbeddingCost` | PROPOSE_ONLY | changes Embedder interface + all impls + worker + soak fakes — not tiny → `fix/embedding-cost-usage-wiring` |
| Per-org token budget guard | PROPOSE_ONLY | needs counters/store → `fix/llm-model-routing-and-budget-guards` |
| Hardcoded spam/short-text prefilter | REJECTED_FOR_SCOPE_OR_SAFETY | would silently drop leads; must be config-driven/auditable |
| Model downgrade on any path | REJECTED_FOR_SCOPE_OR_SAFETY | classifier already mini; comment path must stay `gpt-4.1` |
| Token cap on classifier | REJECTED_FOR_SCOPE_OR_SAFETY | could truncate the JSON verdict → fail-closed dropped lead |
| Distributed/Redis cache, semantic cache, distillation, micro-batching | REJECTED_FOR_SCOPE_OR_SAFETY | explicitly out of Phase-1 scope |

## Intentionally NOT implemented (with follow-ups)

- **Hardcoded prefilter** → `design/dynamic-classifier-prefilter-rules`. Future rules (empty text, exact
  duplicate post, pure emoji, platform boilerplate, known spam phrases, impossible language) MUST be
  config/admin-reviewable, org-aware, conservative, observable, reversible, and never silently drop a
  potential lead without an audit trail. No lead-dropping rule added now.
- **Micro-batching** → `design/classifier-microbatching`. Feasibility: batching N posts per request cuts
  per-call overhead, but raises output-JSON validation risk, needs per-item fallback so one bad item
  cannot poison the batch, bounded batch size (~10–20), added latency, per-item usage attribution when
  the API returns only per-request usage, and per-org isolation. Defer.
- **Semantic cache** → `design/classifier-semantic-cache`. Similar posts can carry different intent;
  requires a high precision threshold, must force a live classify / `needs_review` when uncertain, must be
  org/profile-aware, and must be monitored for false positives. Not Phase 1.
- **Distillation** → `design/classifier-distillation-roadmap`. Needs a labeled dataset, benchmark set,
  confusion matrix, drift monitoring, and a human-review loop; only after enough production data exists.
- **Distributed cache** → `fix/classifier-distributed-cache` (Redis/shared) to recover multi-pod hit-rate.
- **Retry/circuit-breaker** → `fix/agentic-retry-cost-circuit-breakers` (error-only compressed retry for
  the self-heal architect; typed-failure breaker). Audit confirmed no classifier/browser→LLM retry storm.

## Retry / circuit-breaker quick check (Step 8)

- Classifier (`callOpenAIStrictJSON`) makes a **single** attempt — no retry loop; `retry_count=0` logged.
- A browser/comment failure (`target_not_reached`, `redirected_feed`, `composer_failed`, `soft_fail`)
  re-queues the **same stored content** and does **not** re-invoke the classifier or comment generation.
- No broad retry framework added.

## Embedding cost wiring (Step 9)

`internal/workspace_knowledge/embedding/openai.go:Embed` decodes `usage.TotalTokens` locally but its
signature returns only `([][]float32, error)` — usage is discarded. `RecordEmbeddingCost`
(`internal/store/knowledge/cost.go`) has **no callers**. Forwarding real embedding tokens therefore
requires an interface change (`port.go`) + all implementations + soak fakes + worker org attribution —
**not tiny**. Deferred to `fix/embedding-cost-usage-wiring`.
