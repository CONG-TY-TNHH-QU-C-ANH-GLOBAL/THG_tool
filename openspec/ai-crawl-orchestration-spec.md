# AI Crawl Orchestration — Unified System Specification

> **Lifecycle status (2026-07-21 spec IA reconciliation): PROPOSAL — not
> current runtime authority** (nothing under `openspec/` is; see
> `specs/domains/platform-foundation/features/runtime-topology/technical.md`).
>
> **Scope**: Defines the architecture, module boundaries, data contracts, and execution flow for the AI-driven task orchestration layer. Supersedes the skill-based `ai-task-orchestration` proposal. Its premise that the consumed infrastructure (browser runtime manager, container port registry, streaming layer, container control plane) "exists" is inaccurate for this repository — those remain unimplemented proposals; only the durable job scheduler exists (`internal/jobs`, different shape). The realized NL→jobs path is the copilot agent/action pipeline (`internal/drivers/copilot`).

---

## 1. Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  INPUT PLANE                                                                    │
│                                                                                 │
│   Telegram Bot ──────────┐                                                      │
│   Web Chat UI ───────────┤──→  TaskParserEndpoint  (POST /api/v1/tasks/parse)  │
│   REST API Client ───────┘                                                      │
└─────────────────────────────────────┬───────────────────────────────────────────┘
                                       │ free-text input
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  AI TASK PARSER                                                                 │
│  internal/ai/task_parser.go                                                     │
│                                                                                 │
│  Input : string (any language)                                                  │
│  Output: TaskJSON (versioned, validated against TaskSchema v1)                  │
│                                                                                 │
│  • LLM call only (GPT-4o structured output mode)                                │
│  • Extracts: intent, entities, keywords, constraints, filters                   │
│  • NEVER executes anything                                                      │
│  • NEVER selects a handler or skill                                             │
│  • Returns ErrParseAmbiguous if confidence < threshold                          │
└─────────────────────────────────────┬───────────────────────────────────────────┘
                                       │ TaskJSON (schema v1)
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  JOB GENERATOR                                                                  │
│  internal/ai/job_generator.go                                                   │
│                                                                                 │
│  • Validates TaskJSON against TaskSchema                                        │
│  • Verifies task_type is registered in TaskRegistry                             │
│  • Calls jobs.Submit(task_type, task_id, taskJSON)                              │
│  • Writes tasks row: status=pending                                             │
│  • Returns (task_id, job_id) to caller                                          │
│  • NEVER executes crawl logic                                                   │
└─────────────────────────────────────┬───────────────────────────────────────────┘
                                       │ jobs.Submit(...)
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  JOB SCHEDULER  (existing — internal/jobs/)                                     │
│  scheduler_jobs table (SQLite)                                                  │
│                                                                                 │
│  • Durable, UNIQUE(type, idempotency_key) guarantees                            │
│  • Worker pool claims jobs via atomic subquery UPDATE                           │
│  • Dispatches to registered TaskHandler via JobHandler interface                │
└─────────────────────────────────────┬───────────────────────────────────────────┘
                                       │ handler.Handle(ctx, job)
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  TASK REGISTRY                                                                  │
│  internal/crawl/registry.go                                                     │
│                                                                                 │
│  task_type          → handler                                                   │
│  ─────────────────────────────────                                              │
│  facebook_crawl     → FacebookCrawlHandler                                      │
│  lead_generation    → LeadGenHandler                                            │
│  visa_research      → VisaResearchHandler                                       │
│                                                                                 │
│  NO logic. Pure map. Implements jobs.JobHandler per entry.                      │
└─────────────────────────────────────┬───────────────────────────────────────────┘
                                       │ handler.Execute(ctx, task, account)
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  HANDLER LAYER  (internal/crawl/handlers/)                                      │
│                                                                                 │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │  FacebookCrawlHandler / LeadGenHandler / VisaResearchHandler             │  │
│  │                                                                          │  │
│  │  1. Unmarshal CrawlPlan + Filters from task payload                     │  │
│  │  2. Acquire browser container via BrowserRuntimeManager                 │  │
│  │  3. For each source in crawl_plan.sources:                              │  │
│  │     a. Navigate to source                                               │  │
│  │     b. Fetch page items (posts / profiles / comments)                   │  │
│  │     c. Apply filters IN-CRAWL (not post-processing)                    │  │
│  │     d. Deduplicate against existing DB records                          │  │
│  │     e. Accumulate into OutputDataset                                    │  │
│  │  4. Run optional insight pipeline (scoring, clustering, summary)        │  │
│  │  5. Write OutputDataset to tasks.result_json                           │  │
│  │  6. Return nil → scheduler marks job completed                         │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────┬───────────────────────────────────────────┘
                                       │ StartContainer / StopContainer
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  BROWSER RUNTIME MANAGER  (proposed — internal/browser/)                        │
│                                                                                 │
│  • Manages Docker container FSM per account                                     │
│  • Health probe, restart policy, resource limits                                │
│  • Port Registry for CDP/VNC port leases                                        │
│  • Streaming layer for live view                                                │
└─────────────────────────────────────┬───────────────────────────────────────────┘
                                       │ OutputDataset
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  OUTPUT STORE                                                                   │
│  tasks.result_json (SQLite)                                                     │
│                                                                                 │
│  • Structured dataset (records array + metadata)                                │
│  • Optional insights (lead scores, trends, summary)                             │
│  • Queryable via GET /api/v1/tasks/:id and GET /api/v1/tasks                   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Module Definitions

### 2.1 `internal/ai/task_parser.go` — AI Task Parser

**Role**: The only module permitted to call the OpenAI API for task understanding.

**Responsibilities**:
- Accept a free-text string in any language.
- Call GPT-4o in structured output mode with `TaskSchema v1` as the response format JSON Schema.
- Return a validated `TaskJSON` struct or a typed error.
- Record `prompt_tokens` and `completion_tokens` for cost tracking.

**Prohibited**:
- Calling any handler, scheduler, store, or browser package.
- Returning a handler name, skill name, or execution instruction.
- Making decisions about which account to use.

**Errors**:
- `ErrParseAmbiguous{Clarification string}` — model could not extract a complete task; caller must present `Clarification` to the user and retry.
- `ErrParseUnsupportedIntent{Intent string}` — model identified an intent not mapped in `TaskRegistry`.

---

### 2.2 `internal/ai/job_generator.go` — Job Generator

**Role**: Bridge between a validated `TaskJSON` and the durable job scheduler.

**Responsibilities**:
- Validate `TaskJSON` fields (required keys, non-empty sources, valid time range).
- Verify `task.Intent` exists in `TaskRegistry`.
- Call `jobs.Submit(task.Intent, task.TaskID, marshaledTaskJSON)`.
- Write a `tasks` row with `status="pending"` via `store.CreateTask`.
- Return `(taskID, jobID)`.

**Prohibited**:
- Calling the OpenAI API.
- Executing any crawl logic.
- Writing to any table other than `tasks` (no direct writes to `posts`, `leads`, etc.).

---

### 2.3 `internal/crawl/registry.go` — Task Registry

**Role**: Static map from `task_type` string to `TaskHandler` implementation.

**Structure**:
```go
type TaskRegistry struct {
    handlers map[string]TaskHandler
}

type TaskHandler interface {
    Handle(ctx context.Context, task TaskJSON, accountID int64) (*OutputDataset, error)
}
```

**Registered entries (initial)**:

| task_type | Handler |
|---|---|
| `facebook_crawl` | `FacebookCrawlHandler` |
| `lead_generation` | `LeadGenHandler` |
| `visa_research` | `VisaResearchHandler` |

**Prohibited**:
- Any conditional logic inside the registry.
- Any I/O, DB access, or network calls from within registry methods.
- Auto-discovery or dynamic registration after startup.

---

### 2.4 `internal/crawl/handlers/` — Handler Layer

**Role**: The only layer permitted to execute crawling logic.

Each handler receives a fully-validated `TaskJSON` and is responsible for:
1. Unpacking `CrawlPlan` and `Filters` from the task.
2. Acquiring a browser container via `BrowserRuntimeManager.StartContainer`.
3. Iterating `crawl_plan.sources` and applying `filters` **in-crawl** (per-item, before accumulation).
4. Deduplicating results against the `posts` / `leads` / `profiles` tables.
5. Applying the batching strategy defined in `task.Batching`.
6. Running the insight pipeline if `task.Output.Insights` is non-empty.
7. Writing the `OutputDataset` to `store.CompleteTask`.

**Prohibited**:
- Calling the OpenAI API directly (insights use a separate `internal/ai/insight_pipeline.go`).
- Parsing natural language.
- Modifying the `TaskJSON` struct after receipt.
- Calling other handlers (no handler chaining).

---

### 2.5 `internal/ai/insight_pipeline.go` — Insight Pipeline

**Role**: Post-crawl enrichment only. Called by handlers after the clean dataset is assembled.

**Responsibilities**:
- Accept an `OutputDataset` and the `task.Output.Insights` list.
- For each requested insight: call GPT-4o with the dataset subset and return the enrichment.
- Supported insights: `lead_scoring`, `trend_detection`, `clustering`, `summary`.

**Prohibited**:
- Executing any browser or crawl logic.
- Writing to any DB table directly — it returns enriched data; the handler writes it.

---

## 3. Task Schema (v1)

```json
{
  "schema_version": "1",
  "task_id": "<sha256(intent + sorted_entities + iso_date_created)[:16]>",
  "intent": "facebook_crawl | lead_generation | visa_research",
  "created_by": {
    "account_id": 0,
    "org_id": 0
  },
  "crawl_plan": {
    "sources": [
      {
        "type": "facebook_group | facebook_post | facebook_profile | web_url",
        "url": "https://...",
        "label": "optional human label"
      }
    ],
    "query": {
      "keywords": ["từ khóa 1", "từ khóa 2"],
      "exclude_keywords": ["từ không muốn"]
    },
    "time_range": {
      "from": "2026-01-01T00:00:00Z",
      "to": "2026-04-26T23:59:59Z"
    },
    "sampling": {
      "max_items_per_source": 100,
      "max_total_items": 500
    }
  },
  "filters": {
    "must_have": [
      "keyword present in content",
      "author has profile photo"
    ],
    "exclude": [
      "content contains spam phrase",
      "author is page not person"
    ],
    "engagement": {
      "min_comments": 0,
      "min_reactions": 0,
      "min_shares": 0
    },
    "language": ["vi", "en"]
  },
  "batching": {
    "strategy": "sequential | parallel",
    "batch_size": 10,
    "inter_batch_delay_ms": 2000
  },
  "output": {
    "format": "structured_dataset",
    "insights": ["lead_scoring", "trend_detection", "clustering", "summary"]
  }
}
```

### Schema validation rules

| Field | Rule |
|---|---|
| `task_id` | Required; 16-char hex; used as `idempotency_key` in `scheduler_jobs` |
| `intent` | Must match a registered `task_type` in `TaskRegistry` |
| `crawl_plan.sources` | At least one source required |
| `filters.language` | Must be valid BCP-47 codes |
| `batching.strategy` | Must be `sequential` or `parallel` |
| `output.insights` | Each element must be in `{lead_scoring, trend_detection, clustering, summary}` |
| All URL fields | Must start with `https://`; no `localhost`, no private IPs |

---

## 4. Output Dataset Schema

```json
{
  "task_id": "...",
  "task_type": "facebook_crawl",
  "generated_at": "2026-04-26T15:30:00Z",
  "dataset": {
    "records": [
      {
        "id": "<dedup_hash>",
        "source_url": "https://...",
        "source_type": "facebook_group",
        "author": {
          "name": "...",
          "profile_url": "https://...",
          "is_person": true
        },
        "content": "...",
        "timestamp": "2026-04-20T10:00:00Z",
        "engagement": {
          "reactions": 12,
          "comments": 3,
          "shares": 1
        },
        "filter_pass_signals": ["keyword:ship hàng", "min_reactions:met"],
        "language": "vi"
      }
    ],
    "stats": {
      "total_fetched": 200,
      "total_passed_filter": 47,
      "total_deduplicated": 5,
      "total_returned": 42
    }
  },
  "insights": {
    "lead_scores": [
      { "record_id": "...", "score": 0.87, "reason": "high engagement + keyword density" }
    ],
    "trends": [
      { "keyword": "ship hàng mỹ", "frequency": 14, "rising": true }
    ],
    "clusters": [
      { "label": "Hàng mỹ phẩm", "record_ids": ["...", "..."] }
    ],
    "summary": "47 bài viết phù hợp từ 3 nhóm. Chủ đề chính: ship hàng Mỹ. 12 leads tiềm năng cao."
  },
  "token_usage": {
    "parse_prompt_tokens": 420,
    "parse_completion_tokens": 85,
    "insight_prompt_tokens": 1200,
    "insight_completion_tokens": 310
  }
}
```

**Rules**:
- `records` array contains only items that passed all filters. No raw dumps.
- `stats.total_fetched` is the unfiltered fetch count for observability; it is NOT returned to callers — only `total_returned` is user-facing.
- `filter_pass_signals` is a debug field listing which filter conditions the record satisfied.
- `insights` is an empty object `{}` if `task.Output.Insights` was empty.

---

## 5. Handler Responsibilities

### 5.1 `FacebookCrawlHandler`

**Job type**: `facebook_crawl`

**Execution sequence**:
```
1. Unmarshal TaskJSON from job.Payload
2. BrowserRuntimeManager.StartContainer(accountID, orgID)
   → wait for browser_containers.state = 'running' (30s timeout)
3. For each source in crawl_plan.sources:
   a. Navigate to source.url
   b. Loop (fetch page → iterate items):
      i.  For each item:
          - Apply language filter (skip if not in filters.language)
          - Apply keyword filter (skip if no keyword match in content)
          - Apply exclude filter (skip if any exclude condition matches)
          - Apply engagement filter (skip if below thresholds)
          - If all pass: dedup check (sha256 of content+author_url)
          - If not duplicate: append to records
      ii. If records count >= sampling.max_items_per_source: stop source
      iii. If records count >= sampling.max_total_items: stop all sources
      iv. Sleep inter_batch_delay_ms between page fetches
4. Run InsightPipeline(records, task.Output.Insights)
5. store.CompleteTask(taskID, OutputDataset, tokenUsage)
6. Notify caller (Telegram reply or webhook)
```

**Filter application rule**: Filters are evaluated **item-by-item at fetch time**, not on the complete result set. An item failing any filter is discarded immediately and never accumulates in memory.

---

### 5.2 `LeadGenHandler`

**Job type**: `lead_generation`

**Extends** `FacebookCrawlHandler` crawl behavior with:
- After accumulation: calls `InsightPipeline` with `lead_scoring` mandatory.
- Writes qualified leads (`score > LEAD_SCORE_THRESHOLD`) to the `leads` table as `status='pending_review'`.
- Returns the full `OutputDataset` including `lead_scores`.

**Does NOT**: call the AI comment generator, send messages, or post comments. Lead generation is read-only.

---

### 5.3 `VisaResearchHandler`

**Job type**: `visa_research`

**Sources**: `web_url` type sources (not Facebook-specific). Uses the browser container to navigate public government/forum pages.

**Crawl plan specifics**:
- Extracts structured data: deadline dates, fee amounts, document requirements.
- `output.format` is always `structured_dataset`; no social engagement fields.
- `InsightPipeline` called with `summary` and `trend_detection` insights.

---

## 6. Execution Flow (Strict)

### 6.1 Normal path

```
[1]  User sends text input (Telegram / web chat / API)
       │
[2]  AITaskParser.Parse(text, orgID, accountID)
       │ → LLM call → TaskJSON (validated)
       │ → ErrParseAmbiguous: return clarification to user, END
       │ → ErrParseUnsupportedIntent: return error, END
       │
[3]  JobGenerator.Submit(taskJSON)
       │ → Validate schema
       │ → Verify intent in TaskRegistry
       │ → store.CreateTask(status=pending)
       │ → jobs.Submit(intent, task_id, taskJSON)
       │   → INSERT OR IGNORE scheduler_jobs
       │   → return existing row if duplicate (idempotent)
       │ → return (task_id, job_id) to caller → HTTP 202 / Telegram ack
       │
[4]  Scheduler worker claims job (async)
       │ → atomic UPDATE scheduler_jobs: pending → running
       │ → TaskRegistry.Get(task_type) → handler
       │ → store.StartTask(task_id)
       │
[5]  Handler.Handle(ctx, task, accountID)
       │ → BrowserRuntimeManager.StartContainer(accountID, orgID)
       │ → Execute crawl_plan with in-crawl filtering
       │ → InsightPipeline (if requested)
       │ → store.CompleteTask(task_id, outputDataset)
       │ → Notify caller
       │
[6]  Scheduler marks job completed
       → scheduler_jobs.status = 'completed'
```

### 6.2 Failure paths

```
Parse failure (LLM returns ambiguous):
  AITaskParser → ErrParseAmbiguous
  → caller sends clarification message to user
  → no task row, no job row created

Browser start timeout (30s):
  Handler → BrowserRuntimeManager returns timeout error
  → store.FailTask(task_id, "browser start timeout")
  → scheduler retry policy: attempt < max_attempts → reset to pending + run_after
  → after max_attempts: scheduler_jobs.status = 'failed'
  → Telegram error notification

Crawl error (navigation failure, selector failure):
  Handler → internal error
  → store.FailTask(task_id, err)
  → scheduler retry policy applies
  → after max_attempts: final 'failed' state

Handler crash (process restart mid-execution):
  scheduler_jobs stale recovery: claimed_at < now - CLAIMED_TIMEOUT
  → reset to 'pending'
  → new worker claims and re-executes from beginning
  → tasks row reset to 'running' on re-claim
  → OutputDataset is built fresh (deduplification prevents DB duplicates)
```

### 6.3 Idempotency guarantees

| Boundary | Mechanism |
|---|---|
| Duplicate user command | `task_id` = deterministic hash of (intent + entities + date); `UNIQUE(type, idempotency_key)` in `scheduler_jobs` |
| Duplicate Telegram message | Same `task_id` derived from same text → `INSERT OR IGNORE` no-ops |
| Handler restart mid-crawl | Dedup hash in `posts`/`leads` tables prevents re-insertion of already-written records |
| Concurrent submissions | SQLite exclusive write lock on claim UPDATE; one worker wins per job |

---

## 7. Multi-Tenant and Safety Constraints

### 7.1 Org isolation

- `TaskJSON.created_by.org_id` is injected by `JobGenerator` from the authenticated caller's context — it is NEVER sourced from the user's text input.
- All store queries in handlers are parameterized on `org_id`.
- `LeadGenHandler` writes leads with the task's `org_id`; leads from Org A are never visible to Org B.

### 7.2 Source URL validation

The `JobGenerator` validates every URL in `crawl_plan.sources` before submitting the job:
- Must be `https://`
- Must not resolve to `localhost`, `127.0.0.1`, or RFC-1918 private ranges
- Must not be a file:// or data:// URI

A task with any invalid source URL is rejected before any job row is created.

### 7.3 Cross-account data isolation

`accountID` is determined by the `JobGenerator` from the caller's authenticated session, not from the `TaskJSON`. The AI parser never produces an `account_id` — it is always resolved server-side.

---

## 8. Anti-Patterns (MUST NOT EXIST)

| Anti-pattern | Why prohibited |
|---|---|
| Skills, SkillRegistry, SkillRouter in any form | Skills are UI-level automation abstractions; the orchestration layer must not know about click/type/selector logic |
| AI parser calling any handler, scheduler, or store | Parser is read-only; executing from the parsing phase creates non-deterministic side effects |
| Handler calling the AI parser | Handlers receive a fully-specified `TaskJSON`; they do not re-interpret intent |
| Post-processing filters (filter after full fetch) | Fetching and discarding wastes browser time and memory; filters are in-crawl or not at all |
| Raw scraping dump in `result_json` | The output contract requires structured records; any response with unfiltered raw HTML or `[]interface{}` blobs violates the output schema |
| Direct `scheduler_jobs` writes from outside `internal/jobs/` | Job submission goes through `jobs.Submit`; no package bypasses the idempotency layer |
| Handler chaining (handler A calls handler B) | Each handler is an independent execution unit; fan-out is achieved by submitting multiple tasks at the `JobGenerator` level |
| Duplicate task submission systems | `JobGenerator` is the single submission point; no other code path calls `jobs.Submit` for crawl tasks |

---

## 9. Module Boundaries (Import Rules)

```
internal/ai/task_parser      → internal/config, openai client
internal/ai/job_generator    → internal/jobs, internal/store, internal/crawl/registry (read-only)
internal/ai/insight_pipeline → internal/config, openai client
internal/crawl/registry      → (no imports — pure map)
internal/crawl/handlers      → internal/browser, internal/store, internal/ai/insight_pipeline, internal/config
internal/server              → internal/ai/task_parser, internal/ai/job_generator, internal/store
internal/jobs                → internal/store, internal/config
internal/browser             → internal/jobs (Submit only), internal/store, internal/config
```

**Prohibited imports**:
- `internal/ai/task_parser` must NOT import `internal/crawl` or `internal/browser`
- `internal/crawl/registry` must NOT import anything from `internal/`
- `internal/crawl/handlers` must NOT import `internal/ai/task_parser` or `internal/ai/job_generator`
- `internal/jobs` must NOT import `internal/crawl` or `internal/browser`

---

## 10. Database Schema Additions

### `tasks` table

```sql
CREATE TABLE IF NOT EXISTS tasks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id         TEXT    NOT NULL UNIQUE,           -- idempotency key (hex hash)
    org_id          INTEGER NOT NULL DEFAULT 0,
    account_id      INTEGER NOT NULL,
    intent          TEXT    NOT NULL,                   -- facebook_crawl | lead_generation | ...
    task_json       TEXT    NOT NULL,                   -- full TaskJSON (schema v1)
    status          TEXT    NOT NULL DEFAULT 'pending', -- pending|running|completed|failed
    result_json     TEXT,                               -- OutputDataset JSON on completion
    error           TEXT,                               -- error string on failure
    parse_prompt_tokens    INTEGER NOT NULL DEFAULT 0,
    parse_completion_tokens INTEGER NOT NULL DEFAULT 0,
    insight_prompt_tokens  INTEGER NOT NULL DEFAULT 0,
    insight_completion_tokens INTEGER NOT NULL DEFAULT 0,
    duration_ms     INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_org_intent_status
    ON tasks (org_id, intent, status, created_at DESC);
```

### No new tables for crawl output

Crawl records are written to the existing `posts` and `leads` tables (already present). The `tasks.result_json` field stores the structured `OutputDataset` including the filtered record set and insights. Raw browser state is never persisted.

---

## 11. Configuration Variables

| Variable | Default | Description |
|---|---|---|
| `CRAWL_TASK_WORKER_COUNT` | `2` | Dedicated worker slots for crawl tasks (subset of `JOB_WORKER_COUNT`) |
| `CRAWL_BROWSER_START_TIMEOUT` | `30s` | Max wait for container to reach `running` before task fails |
| `CRAWL_INTER_BATCH_DELAY_MS` | `2000` | Default delay between page fetches (overridden by task's `batching.inter_batch_delay_ms`) |
| `CRAWL_MAX_ITEMS_HARD_LIMIT` | `1000` | Absolute cap on `sampling.max_total_items` regardless of task value |
| `LEAD_SCORE_THRESHOLD` | `0.7` | Minimum insight score for a record to be written to `leads` table by `LeadGenHandler` |
| `OPENAI_MODEL` | `gpt-4o` | Model used by both `AITaskParser` and `InsightPipeline` |
| `CRAWL_TASK_RETRY_DELAY` | `60s` | Constant backoff for failed crawl jobs |
| `CRAWL_TASK_MAX_ATTEMPTS` | `2` | Max retry count for `facebook_crawl` and `lead_generation` job types |
