# Workspace Knowledge OS — Backend Technical Architecture

**Status:** Foundation skeleton — Phase A (per `.claude/Architecting the Multi-Tenant Workspace Knowledge OS.md`).
**Stack:** Go + SQLite (MVP). Pgvector / Weaviate / Qdrant deferred to a port-swap (see §6).
**Tenancy model:** Strict org isolation. Every query is `WHERE org_id = ?` — there is no escape hatch.

This document describes the durable backend for the Workspace Knowledge Hub. The UI (`/knowledge`) is the consumer; this document is the contract the backend honors.

---

## 1. Why this exists

The previous design injected a freeform business-profile string into every AI prompt. That approach has four structural problems:

1. **Token waste.** A 4 KB business description is re-sent on every API call. The system pays for the same bytes thousands of times per day.
2. **No structured retrieval.** The AI cannot answer "which of our SKUs matches this lead?" because the catalog isn't a queryable object — it's a paragraph.
3. **No tenancy.** A workspace's offerings, CTAs, and banned claims must live in a database keyed by `org_id`, not a global system prompt.
4. **No observability.** When a comment converts, we can't tell *which* asset earned the conversion. The feedback loop is broken.

The Knowledge OS replaces prompt-string injection with a retrieval-augmented generation (RAG) pipeline: structured assets in a tenant-scoped table → semantic retrieval at runtime → top-K context injection → outcome tracking.

---

## 2. Layered architecture

Seven layers. Each layer has one job; cross-layer talk happens through a single, named contract. This matters because the team will replace specific layers (e.g., SQLite → Postgres + pgvector) without touching the rest.

```
┌─────────────────────────────────────────────────────────────────────┐
│  L7  Observability        retrieval_metrics, sync_metrics          │
├─────────────────────────────────────────────────────────────────────┤
│  L6  Agent Runtime        comment / inbox / post generation         │
│         ↓ requests top-K context from L4 via Searcher port          │
├─────────────────────────────────────────────────────────────────────┤
│  L5  Context Assembly     prompt_injector — top-K → system prompt   │
├─────────────────────────────────────────────────────────────────────┤
│  L4  Retrieval Engine     Searcher port (vector DB swap point)      │
│         ↓ reads from L3                                             │
├─────────────────────────────────────────────────────────────────────┤
│  L3  Knowledge Assets     workspace_knowledge/assets + store        │
│         ↑ normalized from L2                                        │
├─────────────────────────────────────────────────────────────────────┤
│  L2  Ingestion Pipeline   Ingestor port (CSV / Shopify / Notion …)  │
│         ↑ pulls from L1                                             │
├─────────────────────────────────────────────────────────────────────┤
│  L1  Knowledge Sources    workspace_knowledge/sources + store       │
│         ↑ operator-configured                                       │
└─────────────────────────────────────────────────────────────────────┘
```

**Layer interfaces are stable; layer implementations are not.** A future engineer can rewrite L4 from "LIKE-based naive search" to "pgvector ANN" without changing L3, L5, or L6 — provided the new implementation satisfies `Searcher`.

---

## 3. Code layout

```
internal/
  store/                                # Persistence — house style
    schema.go                           # ADD: knowledge_sources, knowledge_assets
    knowledge_sources.go                # NEW: GetForOrg / Upsert / ListForOrg / DeleteForOrg
    knowledge_assets.go                 # NEW: same shape for assets
    knowledge_sources_test.go           # cross-org leak guard
    knowledge_assets_test.go            # cross-org leak guard
  workspace_knowledge/                  # Domain layer — new
    doc.go                              # package overview
    sources/
      types.go                          # Source, SourceType, SyncPolicy, HealthStatus
    assets/
      types.go                          # Asset, AssetType, AssetState, Metric
      normalize.go                      # Raw blob → Asset (canonicalises tags, trims)
    ingestion/
      port.go                           # Ingestor interface
                                        # (implementations land in Phase B)
    retrieval/
      port.go                           # Searcher interface
                                        # (implementations land in Phase C)
    observability/
      metrics.go                        # Retrieval / sync counters (Layer 7 stub)
```

**Why this split:**
- `internal/store/` keeps the house style: monolithic schema, repository methods on `*Store`, integer PKs, JSON-as-TEXT, status enums.
- `internal/workspace_knowledge/` is the *domain* — types, ports, normalizers. It has zero `database/sql` imports. This is what `feedback_contracts_not_orm.md` is asking for: cross-boundary shapes are not DB row serialisations.

The `store` package consumes the domain types from `workspace_knowledge` and translates to/from rows. The handlers and the runtime consume the domain types. The boundary is one-way: domain has no idea persistence exists.

---

## 4. Database schema

### `knowledge_sources`

| Column | Type | Notes |
|---|---|---|
| `id` | `INTEGER PRIMARY KEY AUTOINCREMENT` | House style |
| `org_id` | `INTEGER NOT NULL` | Tenancy boundary |
| `type` | `TEXT NOT NULL` | `shopify` \| `csv` \| `google_sheets` \| `notion` \| `website` \| `catalog` |
| `label` | `TEXT NOT NULL` | Operator-facing name |
| `connection_config` | `TEXT NOT NULL DEFAULT '{}'` | JSON: type-specific creds, URLs, schedule overrides |
| `sync_policy` | `TEXT NOT NULL DEFAULT 'manual'` | `realtime` \| `hourly` \| `daily` \| `manual` |
| `health_status` | `TEXT NOT NULL DEFAULT 'healthy'` | `healthy` \| `syncing` \| `stale` \| `error` \| `needs_auth` |
| `health_message` | `TEXT NOT NULL DEFAULT ''` | Last error / warning text |
| `last_sync_at` | `DATETIME` | NULL until first successful sync |
| `last_sync_asset_count` | `INTEGER NOT NULL DEFAULT 0` | Cached metric |
| `created_at` | `DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP` | |
| `updated_at` | `DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP` | Updated on upsert |

Indexes:
- `idx_knowledge_sources_org` ON (`org_id`, `health_status`) — for the Sources panel hot path.
- `idx_knowledge_sources_sync` ON (`sync_policy`, `last_sync_at`) — for the scheduler picking next-to-sync.

### `knowledge_assets`

| Column | Type | Notes |
|---|---|---|
| `id` | `INTEGER PRIMARY KEY AUTOINCREMENT` | House style |
| `org_id` | `INTEGER NOT NULL` | Tenancy boundary |
| `source_id` | `INTEGER NOT NULL` | FK → `knowledge_sources.id` |
| `external_id` | `TEXT NOT NULL DEFAULT ''` | Stable ID from source (Shopify product GID, CSV row hash, etc.). Used for idempotent upsert. |
| `type` | `TEXT NOT NULL` | `POD_product` \| `faq` \| `shipping_policy` \| `sales_playbook` \| `pricing_rule` \| `banned_claim` \| `cta` |
| `title` | `TEXT NOT NULL` | |
| `description` | `TEXT NOT NULL DEFAULT ''` | |
| `tags` | `TEXT NOT NULL DEFAULT '[]'` | JSON array of strings |
| `payload` | `TEXT NOT NULL DEFAULT '{}'` | JSON: variants, price, image URL, type-specific fields |
| `state` | `TEXT NOT NULL DEFAULT 'pending'` | `pending` \| `approved` \| `hidden` |
| `pinned` | `INTEGER NOT NULL DEFAULT 0` | 0 / 1 — operator force-to-top |
| `boost` | `INTEGER NOT NULL DEFAULT 0` | 0..100 — operator-controlled rank lift |
| `retrieval_count_30d` | `INTEGER NOT NULL DEFAULT 0` | Rolling 30d (refreshed by L7 job) |
| `conversion_count_30d` | `INTEGER NOT NULL DEFAULT 0` | Rolling 30d |
| `last_retrieved_at` | `DATETIME` | NULL until first retrieval |
| `created_at` | `DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP` | |
| `updated_at` | `DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP` | |

Indexes:
- `idx_knowledge_assets_org_state` ON (`org_id`, `state`) — Product Explorer filter chips.
- `idx_knowledge_assets_org_source` ON (`org_id`, `source_id`) — used by the "list assets from this source" panel + cascade on source delete.
- `UNIQUE INDEX uq_knowledge_assets_idem` ON (`org_id`, `source_id`, `external_id`) WHERE `external_id != ''` — **idempotent ingestion** (re-sync updates, does not duplicate).
- `idx_knowledge_assets_org_pin_boost` ON (`org_id`, `pinned` DESC, `boost` DESC, `retrieval_count_30d` DESC) — default sort.

**Why these indexes:** every index is justified by a concrete read path the UI executes today. We are not pre-optimising for hypothetical queries.

---

## 5. Domain types (Go)

Located in `internal/workspace_knowledge/`. **No `database/sql` imports.** These are the cross-boundary contracts.

```go
// sources/types.go
type Source struct {
    ID               int64
    OrgID            int64
    Type             SourceType
    Label            string
    ConnectionConfig json.RawMessage
    SyncPolicy       SyncPolicy
    Health           Health        // value type; bundles status + message + last_sync_at
    LastAssetCount   int
    CreatedAt        time.Time
    UpdatedAt        time.Time
}

type SourceType string  // shopify | csv | google_sheets | notion | website | catalog
type SyncPolicy string  // realtime | hourly | daily | manual

type Health struct {
    Status     HealthStatus
    Message    string
    LastSyncAt *time.Time     // nil until first successful sync — DO NOT default to zero time
}
type HealthStatus string     // healthy | syncing | stale | error | needs_auth
```

```go
// assets/types.go
type Asset struct {
    ID                 int64
    OrgID              int64
    SourceID           int64
    ExternalID         string         // empty allowed (CSV without stable IDs); non-empty enforces idempotence
    Type               AssetType
    Title              string
    Description        string
    Tags               []string       // already deduped + lower-cased by the normalizer
    Payload            json.RawMessage
    State              AssetState
    Pinned             bool
    Boost              int            // clamped 0..100 at the boundary
    Metrics            AssetMetrics   // 30d retrievals/conversions, last_retrieved_at — read-only
    CreatedAt          time.Time
    UpdatedAt          time.Time
}

type AssetMetrics struct {
    Retrievals30d    int
    Conversions30d   int
    LastRetrievedAt  *time.Time       // nil = never retrieved
}
```

**Three deliberate choices:**

1. **`json.RawMessage` for `ConnectionConfig` and `Payload`** — the domain layer doesn't know what's inside; it just hands it through. The per-source-type unmarshal happens in the ingestor that owns that type.
2. **`*time.Time` for `LastSyncAt` / `LastRetrievedAt`** — nil means "never," and that is materially different from "Unix epoch zero." The codebase has been bitten by `time.IsZero()` checks before (`feedback_no_implicit_business_meaning.md`); pointer-or-nil is the explicit branch.
3. **Metrics is a value type, not separate getter** — but the repository never lets you UPDATE these fields through the same path as operator fields. Operator-controlled (`Pinned`, `Boost`, `State`) and system-derived (`Metrics`) write through different repository methods. See §7.

---

## 6. Repository contracts

The `*store.Store` gets these methods. Naming follows the `GetXForOrg` house pattern.

### Sources
```go
GetKnowledgeSource(ctx, sourceID, orgID int64) (*sources.Source, error)
ListKnowledgeSourcesForOrg(ctx, orgID int64) ([]*sources.Source, error)
UpsertKnowledgeSource(ctx, src *sources.Source) (*sources.Source, error)
UpdateKnowledgeSourceHealth(ctx, sourceID, orgID int64, h sources.Health, lastAssetCount int) error
DeleteKnowledgeSourceForOrg(ctx, sourceID, orgID int64) error  // cascades to assets
```

### Assets
```go
GetKnowledgeAsset(ctx, assetID, orgID int64) (*assets.Asset, error)
ListKnowledgeAssetsForOrg(ctx, orgID int64, filter assets.ListFilter) ([]*assets.Asset, error)
UpsertKnowledgeAsset(ctx, asset *assets.Asset) (*assets.Asset, error)   // idempotent on (org_id, source_id, external_id)
SetKnowledgeAssetState(ctx, assetID, orgID int64, state assets.AssetState) error
SetKnowledgeAssetPinned(ctx, assetID, orgID int64, pinned bool) error
SetKnowledgeAssetBoost(ctx, assetID, orgID int64, boost int) error      // clamps 0..100
IncrementKnowledgeAssetRetrieval(ctx, assetID, orgID int64) error       // L7 metric hook
DeleteKnowledgeAssetsForSource(ctx, sourceID, orgID int64) (int64, error)
```

**Tenant-isolation invariants:**

1. **Every method takes `orgID` explicitly.** No method reads it from a shared context — the explicit parameter forces the caller to commit to a tenant.
2. **`Get*` returns `sql.ErrNoRows` for foreign org.** Same pattern as `GetAccountForOrg` ([internal/store/identities/accounts.go](../../../../../internal/store/identities/accounts.go)) — a foreign row is observably indistinguishable from "not found." No data leak through "permission denied" timing.
3. **`Set*` / `Update*` include `WHERE org_id = ?`** in the UPDATE clause. A misrouted update against a foreign row silently affects 0 rows; the caller checks `RowsAffected()` and returns ErrNotFound.
4. **`Delete*ForOrg` is the only delete shape.** No `DeleteByID(id)` — the org_id is the gate. Cascade is explicit (sources delete → assets delete) inside a transaction, not via SQL FK ON DELETE CASCADE (we want to count what we deleted, log it, and return the count).

**Operator vs system writes:**

| Field | Write path | Who calls |
|---|---|---|
| `state`, `pinned`, `boost` | `Set*` | Operator UI handler |
| `title`, `description`, `tags`, `payload` | `UpsertKnowledgeAsset` | Ingestor (L2) only |
| `retrieval_count_30d`, `last_retrieved_at` | `IncrementKnowledgeAssetRetrieval` | Retrieval runtime (L4) only |

These never share a SQL statement. The ingestor re-syncing a SKU must **never** overwrite an operator's `pinned=true`. Enforced at the SQL level via column-specific UPDATE clauses.

---

## 7. Ingestion port (L2)

```go
// workspace_knowledge/ingestion/port.go
type Ingestor interface {
    // Type identifies which Source.Type values this ingestor handles.
    // The dispatcher selects one ingestor per source by exact match.
    Type() sources.SourceType

    // Sync pulls fresh data from the external system and writes
    // normalized assets via the provided writer.
    //
    // It MUST be idempotent: re-syncing the same source updates existing
    // assets (matched on external_id), never duplicates. It MUST be
    // org-scoped: every asset it writes carries src.OrgID.
    //
    // Returns the count of assets seen (created + updated), and any
    // health update for the source row (success → healthy, partial →
    // stale + message, hard fail → error + message).
    Sync(ctx context.Context, src *sources.Source, w AssetWriter) (SyncResult, error)
}

type AssetWriter interface {
    // Write upserts one asset under the calling ingestor's source.
    // The writer enforces that asset.OrgID == src.OrgID and that
    // asset.SourceID == src.ID — an ingestor cannot smuggle assets
    // into another tenant or another source.
    Write(ctx context.Context, asset *assets.Asset) error
}

type SyncResult struct {
    AssetsSeen     int        // total assets touched
    AssetsCreated  int        // first time we saw this external_id
    AssetsUpdated  int        // already existed; fields changed
    AssetsRejected int        // failed normalization, see SyncResult.Errors
    Errors         []SyncError
}
```

**Concrete adapters land in Phase B** (per the architecture doc). Each is a single file:

- `csv_ingestor.go` — reads a CSV, computes `external_id` as SHA-1 of canonical row, normalizes columns by name.
- `shopify_ingestor.go` — pulls products via Shopify Admin API, uses GID as `external_id`.
- `google_sheets_ingestor.go` — Sheets API, row index as `external_id` with a column-mapping config.
- `notion_ingestor.go` — Notion API for sales playbook pages, page-ID as `external_id`.
- `website_ingestor.go` — wraps the existing crawler, hashes page content.

**None of these ingestors talk to `*store.Store` directly.** They take an `AssetWriter`. The writer is implemented by a `store`-backed adapter at the runtime boundary. This way:
- Adapters are testable without a database.
- The store package never imports `ingestion`.
- Cross-org enforcement lives in exactly one place: the `AssetWriter` implementation.

---

## 8. Retrieval port (L4)

```go
// workspace_knowledge/retrieval/port.go
type Searcher interface {
    // TopK returns up to k assets that best match the query for this org.
    // Implementations are free to use LIKE-based naive search, FTS5,
    // pgvector ANN, or a hosted embedding service — the contract is
    // identical.
    //
    // Filter narrows by AssetType (e.g., only POD_product) or state
    // (default: approved-only — the runtime never retrieves hidden or
    // pending assets).
    TopK(ctx context.Context, orgID int64, query string, filter SearchFilter, k int) ([]Hit, error)
}

type SearchFilter struct {
    Types  []assets.AssetType  // empty = any
    States []assets.AssetState // empty = approved only
}

type Hit struct {
    Asset *assets.Asset
    Score float64    // 0..1, higher = better. Implementations document their score semantics.
    Reason string    // short human label: "title-match", "tag-match", "semantic-cosine", etc.
}
```

**Why this port matters:** the current MVP can ship a `naive_searcher.go` that does `LOWER(title) LIKE '%...%' OR LOWER(tags) LIKE '%...%'` and ranks by `pinned DESC, boost DESC, length(matched_field) ASC`. Good enough for the first 1k assets. When usage grows, swap to pgvector — same contract, different implementation, zero callers to change.

`Hit.Reason` is required because the Operator Replay surface (UI Layer) needs to explain *why* an asset was retrieved. "semantic-cosine 0.91" is honest in a way that a bare score is not.

---

## 9. Observability port (L7)

```go
// workspace_knowledge/observability/metrics.go
type Metrics interface {
    RecordSync(ctx context.Context, orgID int64, sourceType sources.SourceType, result ingestion.SyncResult, durationMs int64)
    RecordRetrieval(ctx context.Context, orgID int64, query string, hits []retrieval.Hit, generatedAction string)
    RecordOutcome(ctx context.Context, orgID int64, retrievalID string, outcome string) // sent | rejected | converted
}
```

Implementation: writes to `knowledge_metrics` table (added in Phase D), feeds the existing `prompt_logs` join for the Operator Replay UI surface.

The interface lets you wire a no-op Metrics in tests without instrumenting every call site.

---

## 10. The four invariants (load-bearing)

These are the rules a future engineer must NOT break.

1. **Tenant isolation is enforced at the repository layer, not the handler.** Handlers can forget to filter; the repository cannot. Every `Get*` returns `ErrNoRows` for foreign org; every `Set*`/`Update*` has `WHERE org_id = ?`.
2. **Ingestion is idempotent.** Re-syncing the same source MUST update existing assets, never duplicate. Enforced by the `(org_id, source_id, external_id)` UNIQUE INDEX.
3. **Operator state survives re-sync.** An operator's `pinned` / `boost` / `state` settings must NOT be overwritten by ingestor data. Enforced by column-specific UPDATE clauses in `UpsertKnowledgeAsset`.
4. **Retrieval reads approved assets only by default.** `pending` and `hidden` are operator-visible in the Product Explorer but invisible to the runtime. Enforced by `SearchFilter.States` defaulting to `{approved}` when empty.

Each invariant is covered by a regression test in `internal/store/knowledge_assets_test.go`. They are the load-bearing tests — if they break, the system has stopped being multi-tenant or stopped being durable.

---

## 11. Migration path (existing → Knowledge OS)

Today, business positioning lives in `user_context` as `org:{id}:business_profile` / `services` / `target_signals` etc. These are key-value strings.

**Phase A (this skeleton):** new tables exist alongside `user_context`. Nothing reads them yet.
**Phase B:** ingestors land. Operators can connect Shopify / CSV. Assets start populating.
**Phase C:** runtime starts using `Searcher.TopK` instead of `businessContextForOrg` for the catalog portion of comment generation. The freeform `business_profile` field stays for tone / brand voice.
**Phase D:** `Metrics` is wired; Operator Replay surface goes live.
**Phase E:** image attachment, per-lead personalization.

**Backward compatibility:** until Phase C, comment generation behaves exactly as it does today. No flag flips, no migrations of `user_context` data. Knowledge OS is purely additive until the cutover.

---

## 12. Embedding Pipeline (PR-1 Foundation)

The embedding pipeline lives in `internal/workspace_knowledge/embedding/`. It is the **asynchronous** Layer-2.5: ingestors write asset text; the pipeline turns it into vectors; the (future) pgvector Searcher reads vectors. None of the three layers blocks the others.

### Lifecycle (state machine)

```
                 (text changed)
   ┌──────────────────────────────────────┐
   ▼                                      │
pending  ──worker.Tick──▶  generated  ──Upsert──▶ pending
   │
   │ MaxAttempts exceeded
   ▼
 failed  ──ResetEmbeddingFailures──▶  pending
```

Columns on `knowledge_assets`:

| Column | Purpose |
|---|---|
| `embedding_status` | `pending` \| `generated` \| `failed` \| `skipped` |
| `embedding_model_version` | stable model ID (`openai:text-embedding-3-small:v1`); change triggers re-backfill |
| `embedding_generated_at` | NULL until first success |
| `embedding_input_hash` | sha1(title+description+sortedTags); drives change detection |
| `embedding_attempts` | retry counter |
| `embedding_last_error` | last failure for operator debug |
| `embedding` (PG only) | `VECTOR(1536)`; HNSW indexed |

### Components

- **`embedding.Embedder` port** — `Embed(texts) [][]float32`, `ModelVersion()`, `Dimensions()`. Implemented by `OpenAIEmbedder`. Future: local model adapter.
- **`embedding.Worker`** — long-running goroutine with `Tick`/`Run`. Idempotent batches; retry budget per asset; metrics hook.
- **Store hooks** —
  - `markEmbeddingPendingIfTextChanged` after every `UpsertKnowledgeAsset` — fires only when text content changed.
  - `Set*` operator setters (pin/boost/state) deliberately bypass the hook — operator clicks NEVER cause re-embed.

### Boot-time wiring (runbook for the operator)

```go
// In cmd/scraper/main.go after store.New():
emb := embedding.NewOpenAIEmbedder(os.Getenv("EMBEDDING_API_KEY"), "")
worker := embedding.NewWorker(store, emb)
worker.MetricsRecorder = store
go worker.Run(ctx)
```

When `DATABASE_URL` is unset (SQLite dev mode), the worker still runs and updates metadata columns — useful for testing the state machine without OpenAI credentials. Set `EMBEDDING_API_KEY=""` in that case; the worker records permanent failures cleanly (the assets stay in 'pending' or 'failed' depending on policy).

### Concurrency

- **SQLite**: single worker only (SQLite has no vector column; this path is dev / state-machine testing).
- **Postgres**: N replicas safe. Current `ListPendingEmbeddings` does NOT use `FOR UPDATE SKIP LOCKED` — overlapping batches cost one redundant OpenAI call (idempotent for same input) but never produce duplicate or corrupt data.

### Failure modes covered by tests

| Scenario | Test |
|---|---|
| Happy path (pending → generated) | `TestWorker_HappyPath_PendingToGenerated` |
| Recoverable error (rate limit) → retry | `TestWorker_RecoverableError_RecordsAttempts` |
| Permanent error (auth fail) → fail fast | `TestWorker_PermanentError_RecordsAttempts` |
| Idle workspace → no-op | `TestWorker_IdleReturnsZero` |
| MaxAttempts breach → `failed` status | `TestRecordEmbeddingAttempt_FailsAfterMaxAttempts` |
| Hash determinism (tag reorder = same hash) | `TestInputHash_Deterministic` |
| Operator pin/boost/state → no re-embed | `TestSetters_DoNotMarkEmbeddingPending` |
| Unchanged content re-Upsert → stays generated | `TestUpsert_UnchangedContent_PreservesGeneratedStatus` |
| Changed title → flips to pending | `TestUpsert_ChangedTitle_FlipsBackToPending` |
| Cross-org isolation on attempt records | `TestRecordEmbeddingAttempt_ForeignOrgIsIgnored` |
| Reset failed → pending | `TestResetEmbeddingFailures` |
| pgvector text format | `TestPGVectorLiteral` |

### Observability surface

Embedding batch events go into `knowledge_events` with `event_type='embedding_batch'`. `data_json` carries `{batch_size, succeeded, failed, recoverable}`. Operator dashboards aggregate via:

- `EmbeddingStats` (per-org pending/generated/failed/skipped counts)
- Latency: `duration_ms` column on `knowledge_events`
- Failure rate: `failed / batch_size` from the JSON payload

### What this PR DOES NOT do (deferred to PR-2)

- The pgvector Searcher. Vectors exist in DB after PR-1; PR-2 wires the query path.
- Trace `Breakdown.Semantic` field. Stays 0 until PR-2.
- Backfill CLI command. Operators currently set `embedding_status='pending'` via SQL or rely on the natural ingest cycle.

---

## 13. What this document does NOT specify

- **Vector embedding model.** The `Searcher` port is the swap point — pick OpenAI text-embedding-3-small, BGE-base, or domain-fine-tuned at runtime.
- **Sync scheduler details.** Cron strategy, retry backoff, queue depth, job ordering — these are operational concerns, not architectural ones. They live in the `agentloop` runtime.
- **API shapes for the four UI surfaces.** Those are in `specs/API_SPEC.md` (to be added in Phase A.2 when handlers land).
- **Authorization.** Read = any workspace member; write = admin. Enforced at the handler middleware, not the repository (the repository's job is org_id, not role).
- **Banned claim runtime gate.** Compliance enforcement is described in `feedback_battlefield_badge_framing.md` and lives in `internal/ai/outbound_guard.go` — out of scope here.
