# Production Hardening — 10 Goal Disposition

**Status:** 7 goals shipped end-to-end with code + tests. 3 goals (G2 Snapshots, G5 Query Rewrites, G7 Resource Isolation) scaffolded with design + minimum schema; full implementation is multi-PR and is sized below.

**Last update:** 2026-05-18.

This document is the operator-facing dashboard for the 10 production-hardening goals. Each goal has a verdict (READY / DESIGNED / DEFERRED) and a pointer to the load-bearing code or follow-up PR.

---

## Verdict per goal

| # | Goal | Verdict | Load-bearing artefact |
|---|---|---|---|
| **G1** | Evaluation gold dataset + CI gate | ✅ READY | [internal/workspace_knowledge/soak/gold_dataset.go](../../../../../../internal/workspace_knowledge/soak/gold_dataset.go) · [gold_dataset_test.go](../../../../../../internal/workspace_knowledge/soak/gold_dataset_test.go) |
| **G2** | Snapshots — deterministic historical replay | 📐 DESIGNED (§G2 below) | scaffold in this doc; PR work in 3 incremental commits |
| **G3** | Bounded jobs runtime | ✅ READY | [internal/workspace_knowledge/embedding/supervised.go](../../../../../../internal/workspace_knowledge/embedding/supervised.go) · 5 leak-detection tests |
| **G4** | 3-layer governance (Retrieval / Prompt / Output) | ✅ READY | L1 in hybrid+pgvector+rrf · L2 in assembly · L3 in [internal/workspace_knowledge/governance/output_validator.go](../../../../../../internal/workspace_knowledge/governance/output_validator.go) |
| **G5** | Query rewrite policy — deterministic + traced | 📐 DESIGNED (§G5 below) | trace shape ready (`ScoredHit` extensible); no hidden LLM rewrite path exists today (verified by audit) |
| **G6** | RRF authority — semantic never overrides governance/pin | ✅ READY | [retrieval/rrf/rrf_searcher.go](../../../../../../internal/workspace_knowledge/retrieval/rrf/rrf_searcher.go) defence-in-depth gate · [rrf_authority_test.go](../../../../../../internal/workspace_knowledge/retrieval/rrf/rrf_authority_test.go) |
| **G7** | Resource isolation — no cross-tenant starvation | 📐 DESIGNED (§G7 below) | tenant isolation under read-load proven; per-org write quota is the open PR |
| **G8** | Security — sanitize hostile content, redact secrets | ✅ READY | [internal/workspace_knowledge/security/sanitizer.go](../../../../../../internal/workspace_knowledge/security/sanitizer.go) + [redact.go](../../../../../../internal/workspace_knowledge/security/redact.go) · hooked at `UpsertKnowledgeAsset` |
| **G9** | Cost accounting per-org / per-source / 30d rolling | ✅ READY | [internal/store/knowledge/cost.go](../../../../../../internal/store/knowledge/cost.go) |
| **G10** | Human feedback — immutable, no auto-train | ✅ READY | [internal/store/knowledge/feedback.go](../../../../../../internal/store/knowledge/feedback.go) · schema in `schema.go` |

---

## §G2 — Snapshots (deterministic historical replay)

### Requirement

Historical replay must be **100% deterministic**. Given a retrieval event from yesterday, today's replay UI must show EXACTLY what the operator saw yesterday — same hits, same scores, same ranking — regardless of:
- assets that have been updated/added/deleted since
- embeddings regenerated under a new model
- prompts / leads modified

### Why this matters

Without snapshots, "Why did the AI choose this 3 weeks ago?" is unanswerable. Replay becomes a time-shifted re-execution rather than a faithful historical artefact. Operators stop trusting it.

### Design

A retrieval snapshot is a write-once, content-addressed record of:

1. The **query**: lead text, filter, org_id, retrieval_id.
2. The **candidate set considered**: for each asset visible to the searcher, capture {asset_id, title, description, tags, payload, state, pinned, boost, embedding_model_version, embedding_hash}.
3. The **decisions**: which candidates scored what, what was kept, what was dropped, what the breakdown was.
4. The **environment**: searcher_impl, embedder_model_version, ranker config hash.

The existing `retrieval.Trace` covers (1) + parts of (3) + (4). It does NOT cover (2) — the full candidate set with its STATE AT THE TIME — which is what makes replay deterministic.

### Schema (planned migration `0004_add_retrieval_snapshots`)

```sql
CREATE TABLE retrieval_snapshots (
    retrieval_id   TEXT PRIMARY KEY,
    org_id         BIGINT NOT NULL,
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Snapshot envelope: query, filter, environment.
    envelope_json  TEXT NOT NULL,
    -- Candidate set: per-asset {id, title, description, tags JSON,
    -- payload JSON, state, pinned, boost, embedding_model_version}.
    candidates_json TEXT NOT NULL,
    -- Decisions: trace + budget exactly as the runtime produced.
    trace_json     TEXT NOT NULL,
    budget_json    TEXT NOT NULL
);
CREATE INDEX idx_retrieval_snapshots_org ON retrieval_snapshots(org_id, created_at DESC);
```

### Write path

Wrap `runtime.Builder.BuildForLeadWithTrace` so that AFTER retrieval succeeds, the runtime serialises the full candidate set + trace into the snapshot. The asset-state pinning happens at the SQL level — the snapshot captures rows as-read, not as-they-now-are.

### Read path

`GET /api/org/knowledge/snapshots/:retrieval_id` returns the envelope. The Replay UI's "deep dive" view reads this instead of the (potentially evolved) trace alone.

### Determinism guarantee

Replays NEVER call retrieval again. They render the captured envelope. If an asset was deleted yesterday, the snapshot still has its title. If embeddings were regenerated, the snapshot's `embedding_model_version` shows the old one. Nothing about today's state can leak into a yesterday's replay.

### Why DESIGNED, not READY

Three PRs needed:
- **PR-S1** schema + Store write methods (small).
- **PR-S2** wire `Builder.BuildForLeadWithTrace` to snapshot (medium — requires fetching candidate set, not just trace).
- **PR-S3** Read endpoint + UI wiring (small + frontend).

Each is independently shippable and the existing trace continues to work in the interim. No urgency unless replay determinism becomes a load-bearing operator commitment.

---

## §G5 — Query Rewrite Policy

### Requirement

Query rewrites (lead text → search query) must be **deterministic, explainable, and traceable**. Hidden LLM rewriting without a trace is **strictly forbidden**.

### Current state (verified by audit)

**There is no query rewriting today.** Lead text flows directly to:
- `hybrid.Searcher.TopKWithTrace(ctx, orgID, leadText, ...)` — tokens + phrase matching on the raw text.
- `pgvector.Searcher.TopKWithTrace(...)` — embedder hashes the raw text.

No LLM rewrite layer exists. Every prompt that reaches a searcher is the original lead text (truncated to 240 chars by `retrieval.TruncateQuery`). This is the SAFE default and we should preserve it.

### Policy

1. **No hidden rewriting.** If a future feature wants to rewrite a query (e.g. "expand 'POD' to 'print on demand'"), it MUST:
   - Run BEFORE the searcher.
   - Record the rewrite in the trace with `original_query`, `rewritten_query`, and `rewrite_reason`.
   - Be deterministic OR carry the random-seed in the trace.

2. **LLM-based rewrites are restricted.** Allowed only via a documented `QueryRewriter` port (planned PR adds `retrieval/rewriter/port.go`). Hidden LLM calls inside other code paths are forbidden.

3. **Trace shape extension** (already supported additively): the `Trace` struct gains optional `QueryRewrites []QueryRewrite` populated when rewriting occurred. Older traces with no rewrites unmarshal cleanly with an empty slice.

### Why DESIGNED, not READY

Until a query-rewrite feature is requested by a product use case, no code is required — the absence of rewriting IS the compliance. This document is the policy statement; a future PR adding the QueryRewriter port lands the contract.

---

## §G7 — Resource Isolation

### Requirement

One organization must NEVER:
- Starve another org's queries.
- Exhaust the embedding-worker pool such that other orgs can't ingest.
- Block vector ingestion for others.

### Current state

- **Read path (retrieval)**: SQLite holds a single connection pool; concurrent reads share it via `database/sql`. Tenant-isolation under load is PROVEN ([real_soak_test.go `TestRealSoak_TenantIsolationUnderLoad`](../../../../../../internal/workspace_knowledge/soak/real_soak_test.go) — 4 orgs × 10 queries, 0 leaks). Read-path starvation is theoretically possible but unobserved at MVP scale.
- **Write path (embedding worker)**: `Worker.Tick` polls `ListPendingEmbeddings` org-agnostically. A single org with 100k pending assets WOULD monopolise the worker until drained. **THIS IS THE OPEN GAP.**

### Design — per-org fair scheduling

Replace the worker's simple LIMIT pull with a fairness-aware pull:

```sql
-- Conceptually: round-robin across orgs with pending assets.
WITH per_org_pending AS (
    SELECT org_id, MIN(id) AS first_id, COUNT(*) AS cnt
      FROM knowledge_assets
     WHERE embedding_status = 'pending'
     GROUP BY org_id
)
SELECT a.id, a.org_id, ...
  FROM knowledge_assets a
  JOIN per_org_pending p ON a.org_id = p.org_id AND a.id = p.first_id
 ORDER BY p.cnt ASC      -- prioritise orgs with FEWER pending (least-served first)
 LIMIT N;
```

Each Tick processes the first pending asset per org rather than the first N assets globally. Pathological case: 100k-pending org gets one slot per tick, every other org also gets one slot per tick → no starvation.

### Per-org rate limits (future)

When the system grows past dozens of tenants, add a token-bucket per org_id capping embedding-API calls per hour. Cost-tracking (G9) provides the substrate; rate-limiting is the layer above. Defer until tenants × pending volume justifies it.

### Why DESIGNED, not READY

The fair-scheduling SQL is small (one file change) but needs careful testing under contention. Sized as PR-RI1. Until then, the operator monitors `EmbeddingStats` per org and manually pauses orgs that starve others.

---

## Rollout summary

**Goal directive said:** all 10 must hold for the system to be production-trustable.

**This PR ships:** 7 goals fully (G1, G3, G4, G6, G8, G9, G10) — every one with code + tests proving the invariant.

**This PR designs:** 3 goals (G2, G5, G7) — each with a concrete plan sized for follow-up PRs.

**The 3 designed goals are not blockers FOR THE CURRENT WORKLOAD** but ARE blockers before:
- G2 — before "operator must be able to audit last quarter's AI decisions"
- G5 — before any LLM-driven query expansion ships
- G7 — before tenant count grows past ~50 active workspaces

Concrete next step (after this PR merges): implement PR-S1 (Snapshot schema + write methods). It is the largest of the three remaining and has the most downstream dependencies.
