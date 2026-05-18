# Retrieval Substrate Soak Report

**Generated:** 2026-05-18T17:54:31Z
**Searcher:** rrf · **Embedder:** openai:text-embedding-3-small:v1 (1536 dims) · **k=5**

## Operator Trust: **READY** (score: 85/100)

## Catalog
- Total assets: 17
- Embeddings generated: 17 / pending: 0 / failed: 0
- By type:
  - shipping_policy: 2
  - pricing_rule: 1
  - cta: 3
  - banned_claim: 1
  - POD_product: 10

## Retrieval Quality
- Mean Precision@K: 0.62 · Median: 0.80 · P10: 0.00
- Prompts PASS: 10 · FAIL: 0 · DEGRADED: 0
- Avg retrieved per prompt: 4.1
- Latency avg: 6.8ms · p95: 9ms

## Fallback Behaviour
- Fallback rate: 0.0% (0 / 10)

## Replay Health
- Traces complete: 10 / 10 (100.0%)

## Per-Prompt Outcomes

| Lang | Verdict | Score | P@K | Lat ms | Prompt |
|---|---|---|---|---|---|
| en | ✅ PASS | 0.03 | 1.00 | 7 | Looking for custom cat tee POD with US shipping |
| en | ✅ PASS | 0.03 | 0.80 | 9 | Need dog hoodie supplier MOQ 50 |
| en | ✅ PASS | 0.03 | 0.80 | 8 | Need supplier for oversized gothic anime tees in US wholesal… |
| en | ✅ PASS | 0.03 | 0.60 | 8 | Looking for kawaii pastel hoodies for streetwear brand |
| vi | ✅ PASS | 0.02 | 1.00 | 7 | Cần fulfill áo thun mèo cho thị trường Mỹ, giá s… |
| vi | ✅ PASS | 0.03 | 0.80 | 7 | Tìm xưởng POD chó hoodie cho team marketing US |
| en | ✅ PASS | 0.03 | 0.50 | 6 | What is your shipping time to Germany and returns policy? |
| en | ✅ PASS | 0.03 | 0.75 | 6 | Interested in wholesale enquiry |
| en | ✅ PASS | 0.02 | 0.00 | 5 | Are your products best price guaranteed? |
| en | ✅ PASS | 0.02 | 0.00 | 5 | Recommend a good Italian restaurant in Hanoi |

## Compliance / Governance
✅ No banned claims surfaced. No hidden-state assets surfaced.

## Failure Mode Scenarios
- ✅ **A** — Embedder API down: Fallback invoked; hybrid produced 5 hits.
- ✅ **B** — pgvector extension unavailable: Hybrid served directly; trace.SearcherImpl=hybrid-v1
- ✅ **C** — Partial embedding backfill: Hybrid path produced 5 hits independent of vector readiness.
- ✅ **D** — Slow semantic query: Fallback fired within 50.3541ms; reason=fallback_primary_timeout.
- ✅ **E** — Tenant with zero assets: Empty workspace returned 0 hits cleanly; no error.
- ✅ **F** — Stale-only catalog: Stale-asset query returned 0 (catalog fresh; observability works).

## Cost Telemetry (real HTTP exercise)
- Embedding requests: 151 · tokens served: 6607
- Avg tokens / request: 43.8
- Failures: 0 × 429 (rate limit) · 0 × 5xx
- Estimated cost: $0.000132 (text-embedding-3-small @ $0.02/1M tokens)

