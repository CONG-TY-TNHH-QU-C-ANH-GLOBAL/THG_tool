## Architecture Diagram

```
╔══════════════════════════════════════════════════════════════════════════════════╗
║  INPUT PLANE                                                                    ║
║                                                                                 ║
║  Telegram Bot ────────┐                                                         ║
║  Web Chat UI ─────────┤──→ POST /api/v1/tasks/parse   { text, account_id }     ║
║  REST API Client ─────┘                                                         ║
╚═══════════════════════════════════════╦════════════════════════════════════════╝
                                        ║ free-text string
                                        ▼
╔═══════════════════════════════════════════════════════════════════════════════╗
║  [1] AI TASK PARSER          internal/ai/task_parser.go                      ║
║                                                                               ║
║  • GPT-4o structured output mode (response_format = TaskSchema JSON Schema)   ║
║  • Extracts: intent · entities · keywords · crawl_plan · filters              ║
║  • Computes task_id = sha256(intent+sorted_entities+date)[:16]                ║
║  • Returns: TaskJSON (v1) | ErrAmbiguous{msg} | ErrUnsupportedIntent{intent}  ║
║                                                                               ║
║  STATELESS — no DB, no scheduler, no browser calls, no side effects           ║
╚═══════════════════════════════════════╦═══════════════════════════════════════╝
                                        ║ TaskJSON (validated, versioned)
                                        ▼
╔═══════════════════════════════════════════════════════════════════════════════╗
║  [2] JOB GENERATOR           internal/crawl/job_generator.go                 ║
║                                                                               ║
║  • Validates TaskJSON against schema (required fields, URL safety, limits)    ║
║  • Verifies task.Intent exists in TaskRegistry                                ║
║  • store.CreateTask(task_id, intent, org_id, account_id, task_json)           ║
║  • jobs.Submit(type=task.Intent, idempotency_key=task.TaskID, payload)        ║
║    → INSERT OR IGNORE — duplicate task_id returns existing row                ║
║  • Returns: TaskRecord{task_id, job_id, status="pending"}                     ║
║                                                                               ║
║  SINGLE SUBMISSION POINT — no other code calls jobs.Submit for crawl tasks    ║
╚═══════════════════════════════════════╦═══════════════════════════════════════╝
                                        ║ jobs.Submit(intent, task_id, taskJSON)
                                        ▼
╔═══════════════════════════════════════════════════════════════════════════════╗
║  [3] JOB SCHEDULER           internal/jobs/  (existing, unchanged)           ║
║      scheduler_jobs table (SQLite)                                            ║
║                                                                               ║
║  • UNIQUE(type, idempotency_key) constraint enforces exactly-once execution   ║
║  • Atomic claim: UPDATE … WHERE id=(SELECT … status='pending') RETURNING *    ║
║  • Stale recovery: claimed_at < now - CLAIMED_TIMEOUT → reset to pending      ║
║  • Dispatches to: TaskRegistry.Get(task_type).Handle(ctx, job)                ║
╚═══════════════════════════════════════╦═══════════════════════════════════════╝
                                        ║ handler.Handle(ctx, job)
                                        ▼
╔═══════════════════════════════════════════════════════════════════════════════╗
║  [4] TASK REGISTRY           internal/crawl/registry.go                      ║
║                                                                               ║
║  Registered at startup:                                                       ║
║  ┌──────────────────────────────────────────────────────────────────────┐    ║
║  │  "facebook_crawl"   → FacebookCrawlHandler                          │    ║
║  │  "lead_generation"  → LeadGenHandler                                │    ║
║  │  "visa_research"    → VisaResearchHandler                           │    ║
║  │  "web_crawl"        → WebCrawlHandler                               │    ║
║  └──────────────────────────────────────────────────────────────────────┘    ║
║                                                                               ║
║  Pure map. Zero logic. Zero I/O. Zero imports from internal/.                 ║
╚═══════════════════════════════════════╦═══════════════════════════════════════╝
                                        ║ handler.Execute(ctx, taskJSON, accountID)
                                        ▼
╔═══════════════════════════════════════════════════════════════════════════════╗
║  [5] HANDLER LAYER           internal/crawl/handlers/                        ║
║                                                                               ║
║  ┌──────────────────────────────────────────────────────────────────────┐    ║
║  │  FacebookCrawlHandler / LeadGenHandler / VisaResearchHandler /       │    ║
║  │  WebCrawlHandler                                                     │    ║
║  │                                                                      │    ║
║  │  1. store.StartTask(task_id)                                         │    ║
║  │  2. BrowserRuntimeManager.StartContainer(accountID, orgID)           │    ║
║  │  3. Execute CrawlPlan steps (loop over sources):                     │    ║
║  │     ┌────────────────────────────────────────────────────────────┐  │    ║
║  │     │  FETCH LOOP (per source, per page batch)                   │  │    ║
║  │     │                                                            │  │    ║
║  │     │  fetch_batch(source, offset, batch_size)                   │  │    ║
║  │     │      ↓                                                     │  │    ║
║  │     │  for each raw_item in batch:                               │  │    ║
║  │     │      FilterEngine.Evaluate(raw_item, task.Filters)         │  │    ║
║  │     │        PASS → DeduplicateCheck(raw_item)                   │  │    ║
║  │     │                 NEW  → accumulate to records[]             │  │    ║
║  │     │                 DUP  → discard, stats.dedup++              │  │    ║
║  │     │        FAIL → discard immediately, stats.rejected++        │  │    ║
║  │     │                                                            │  │    ║
║  │     │  if records >= sampling.max_total_items: break             │  │    ║
║  │     │  sleep inter_batch_delay_ms                                │  │    ║
║  │     └────────────────────────────────────────────────────────────┘  │    ║
║  │  4. InsightPipeline.Run(records, task.Output.Insights)               │    ║
║  │  5. store.CompleteTask(task_id, OutputDataset)                       │    ║
║  │  6. Notify caller (Telegram / webhook)                               │    ║
║  └──────────────────────────────────────────────────────────────────────┘    ║
╚══════════════╦════════════════════════════╦══════════════════════════════════╝
               ║                            ║
               ▼                            ▼
╔══════════════════════════╗  ╔════════════════════════════════════════════════╗
║  [6] FILTER ENGINE        ║  ║  [7] BROWSER RUNTIME MANAGER  (existing)     ║
║  internal/crawl/filter/  ║  ║  internal/browser/                            ║
║                          ║  ║                                               ║
║  Per-item, synchronous:  ║  ║  • Docker FSM (8 states)                      ║
║  • KeywordRelevance      ║  ║  • ContainerHealthProbe                       ║
║  • SpamDetector          ║  ║  • RestartPolicy                              ║
║  • IntentMatchScorer     ║  ║  • PortRegistry (lease-based)                 ║
║  • QualityThreshold      ║  ║  • StreamingProvider (CDP/VNC)                ║
║  • DuplicateChecker      ║  ║                                               ║
║                          ║  ║  Called ONLY by handlers.                    ║
║  Returns: PASS | FAIL    ║  ║  Never called by parser or generator.         ║
║  FAIL → immediate drop   ║  ╚════════════════════════════════════════════════╝
╚══════════════════════════╝
               ║
               ▼ (PASS only)
╔═══════════════════════════════════════════════════════════════════════════════╗
║  [8] OUTPUT DATASET          tasks.result_json (SQLite)                      ║
║                                                                               ║
║  {                                                                            ║
║    "records": [ ...filtered, deduplicated items only... ],                   ║
║    "stats": { "total_fetched": N, "total_returned": M },                     ║
║    "insights": { "lead_scores": [...], "trends": [...], "summary": "..." }   ║
║  }                                                                            ║
║                                                                               ║
║  NO raw dumps. NO rejected items. NO intermediate state.                     ║
╚═══════════════════════════════════════════════════════════════════════════════╝
```

---

## Module Definitions

### Module 1 — AI Task Parser (`internal/ai/task_parser.go`)

**Input**: `(ctx, text string, orgID int64, accountID int64)`  
**Output**: `(*TaskJSON, ParseTokenUsage, error)`

**Internal flow**:
1. Build system prompt listing supported intents with their expected entities (static, updated when intents change).
2. Call `client.CreateChatCompletion` with `response_format={type:"json_schema", json_schema:{name:"TaskSchema", schema:<v1 JSON Schema>}}`.
3. Unmarshal response into `TaskJSON`.
4. Inject `created_by.org_id` and `created_by.account_id` from server-side context (never from LLM output).
5. Compute `task_id = hex(sha256(intent + sorted_keywords + truncate(text,200) + date_utc))[:16]`.
6. Return `TaskJSON`.

**Error types**:
```go
type ErrAmbiguous struct { Clarification string }
type ErrUnsupportedIntent struct { Intent string }
type ErrSchemaValidation struct { Field string; Reason string }
```

**Invariants**:
- `created_by` fields are ALWAYS set by the parser from server context, never from LLM text.
- The parser does not know what accounts exist. Account resolution is the Job Generator's responsibility.
- The parser is called at most once per user message. No retry loop inside the parser.

---

### Module 2 — Job Generator (`internal/crawl/job_generator.go`)

**Input**: `(ctx, taskJSON *TaskJSON) → (*TaskRecord, error)`

**Validation sequence** (all must pass before any DB write):
1. `task_id` is 16-char hex.
2. `intent` is in `TaskRegistry`.
3. All `crawl_plan.sources[].url` pass URL safety check (https, no private IPs).
4. `sampling.max_total_items <= CRAWL_MAX_ITEMS_HARD_LIMIT`.
5. `filters.language` contains only valid BCP-47 codes.

**Submission sequence** (only runs if validation passes):
```
store.CreateTask(task_id, intent, org_id, account_id, taskJSON) → task row, status=pending
jobs.Submit(task.Intent, task.TaskID, marshaledPayload)
  → INSERT OR IGNORE scheduler_jobs
  → SELECT by (type, idempotency_key) → returns existing row if duplicate
return TaskRecord{task_id, job_id, status}
```

**Idempotency**: Two calls with identical `task_id` produce one `scheduler_jobs` row and one `tasks` row. The second call returns the existing `TaskRecord` unchanged.

---

### Module 3 — Task Registry (`internal/crawl/registry.go`)

```go
type TaskHandler interface {
    Handle(ctx context.Context, job jobs.Job) error
}

type TaskRegistry struct {
    entries map[string]TaskHandler  // set once at startup, read-only thereafter
}

func (r *TaskRegistry) Register(intent string, h TaskHandler)
func (r *TaskRegistry) Get(intent string) (TaskHandler, bool)
func (r *TaskRegistry) All() []string
```

**Rule**: `Register` may only be called before `jobs.Scheduler.Start()`. `Get` is called concurrently by workers; no locking needed after startup because the map is read-only.

---

### Module 4 — Filter Engine (`internal/crawl/filter/`)

```go
type FilterResult struct {
    Pass    bool
    Score   float64
    Signals []string  // which rules contributed ("keyword:ship hàng", "spam:rejected")
}

type FilterEngine struct {
    cfg FilterConfig
}

func (f *FilterEngine) Evaluate(item RawItem, filters TaskFilters) FilterResult
```

**Filter pipeline** (executed in this order; first FAIL short-circuits):

| Step | Component | Logic |
|---|---|---|
| 1 | `KeywordRelevance` | Tokenize `item.content`; score = matched_keywords / total_keywords; FAIL if score < `filters.keyword_relevance_min` (default 0.3) |
| 2 | `SpamDetector` | Pattern match against spam phrase list (`filters.exclude`); FAIL on any match; also FAIL if content is < 20 chars or > 95% uppercase |
| 3 | `EngagementGate` | FAIL if `item.reactions < filters.engagement.min_reactions` OR `item.comments < filters.engagement.min_comments` |
| 4 | `LanguageFilter` | Detect item language (simple n-gram heuristic); FAIL if not in `filters.language` |
| 5 | `IntentMatchScorer` | Compute TF-IDF overlap between item content and task `crawl_plan.query.keywords`; FAIL if score < `CRAWL_INTENT_MATCH_THRESHOLD` (default 0.2) |
| 6 | `QualityThreshold` | FAIL if item has no `author.profile_url` (unattributed content); FAIL if `item.timestamp` is outside `filters.time_range` |

**Deduplication** (separate from filter, runs only on PASS items):
```go
type DuplicateChecker struct { store *store.Store }

func (d *DuplicateChecker) IsNew(item RawItem, orgID int64) bool {
    hash := sha256(item.content + item.author_profile_url)
    // SELECT 1 FROM posts WHERE dedup_hash=? AND org_id=?
    // returns true if no row found
}
```

Dedup uses the existing `posts.dedup_hash` column. No new table.

---

### Module 5 — Handler Layer (`internal/crawl/handlers/`)

All handlers share a base `CrawlBase` struct:

```go
type CrawlBase struct {
    runtime  *browser.BrowserRuntimeManager
    store    *store.Store
    filter   *filter.FilterEngine
    dedup    *filter.DuplicateChecker
    insights *ai.InsightPipeline
    cfg      config.Config
}
```

Each handler embeds `CrawlBase` and implements `TaskHandler.Handle`.

#### `FacebookCrawlHandler`

**Crawl targets**: `facebook_group`, `facebook_post`, `facebook_profile` source types.

**Step sequence**:
```
1. Unmarshal TaskJSON from job.Payload
2. store.StartTask(task_id)
3. BrowserRuntimeManager.StartContainer(accountID, orgID)
   → poll browser_containers.state = 'running', timeout = CRAWL_BROWSER_START_TIMEOUT
4. Acquire CDP session via workspace.Manager.Get(accountID)
5. For each source in crawl_plan.sources:
   a. Navigate to source.url
   b. batch_offset = 0
   c. loop:
      - raw_items = fetch_batch(cdp_session, source, batch_offset, batching.batch_size)
      - if len(raw_items) == 0: break
      - for each raw_item:
          result = FilterEngine.Evaluate(raw_item, task.Filters)
          stats.total_fetched++
          if result.Fail: continue
          if !DuplicateChecker.IsNew(raw_item, orgID): stats.dedup++; continue
          records = append(records, toStructuredRecord(raw_item, result.Signals))
          stats.total_returned++
          if stats.total_returned >= sampling.max_total_items: goto done
      - batch_offset += batching.batch_size
      - sleep(batching.inter_batch_delay_ms)
6. done:
7. insights = InsightPipeline.Run(records, task.Output.Insights)
8. store.CompleteTask(task_id, OutputDataset{records, stats, insights})
9. BrowserRuntimeManager.StopContainer(accountID, orgID)  ← only if started by this handler
10. NotifyCaller(task.CreatedBy, OutputDataset.Stats)
```

**Must NOT**: parse intent, modify TaskJSON, call Docker API directly, call FilterEngine after collection.

#### `LeadGenHandler`

Embeds `FacebookCrawlHandler`. After step 6 (`done`):
- Calls `InsightPipeline.Run` with `lead_scoring` mandatory.
- For each record with `lead_score >= LEAD_SCORE_THRESHOLD`: writes to `leads` table with `status='pending_review'`, `org_id`, `source_task_id`.
- `OutputDataset.insights.lead_scores` contains scores for all records, not just threshold-passing ones.

#### `VisaResearchHandler`

**Crawl targets**: `web_url` source type (government sites, visa agencies).

**Difference from Facebook handler**:
- Extracts structured fields: `title`, `deadline_date`, `fee_amount`, `document_list`, `office_address` using CSS selectors specific to common Vietnamese government page layouts.
- `FilterEngine` rules applied: `KeywordRelevance`, `QualityThreshold` only (no `EngagementGate` — web pages have no reactions/comments).
- `InsightPipeline` called with `summary` and `trend_detection`.

#### `WebCrawlHandler`

**Crawl targets**: Any `web_url` source. Handles POD/dropshipping discovery, general intent search.

**Difference**:
- No Facebook-specific data extraction. Uses generic structured extraction: `title`, `description`, `contact_info`, `price_signals`, `url`.
- Filter pipeline applies `KeywordRelevance`, `SpamDetector`, `IntentMatchScorer` only.

---

## Task Schema (Final — v1)

```json
{
  "schema_version": "1",
  "task_id": "a3f7c2e1b9d40851",
  "intent": "facebook_crawl",
  "created_by": {
    "account_id": 42,
    "org_id": 7
  },
  "crawl_plan": {
    "sources": [
      {
        "type": "facebook_group",
        "url": "https://www.facebook.com/groups/shiphangmy",
        "label": "Ship hàng Mỹ"
      },
      {
        "type": "facebook_group",
        "url": "https://www.facebook.com/groups/orderhang",
        "label": "Order hàng US"
      }
    ],
    "query": {
      "keywords": ["ship hàng mỹ", "order mỹ phẩm", "hàng xách tay"],
      "exclude_keywords": ["spam", "quảng cáo", "mua ngay"]
    },
    "time_range": {
      "from": "2026-04-01T00:00:00Z",
      "to": "2026-04-26T23:59:59Z"
    },
    "sampling": {
      "max_items_per_source": 100,
      "max_total_items": 300
    }
  },
  "filters": {
    "must_have": [],
    "exclude": ["spam", "bot post", "xem thêm"],
    "engagement": {
      "min_reactions": 2,
      "min_comments": 0,
      "min_shares": 0
    },
    "language": ["vi"],
    "keyword_relevance_min": 0.3
  },
  "dedup_rules": {
    "key_fields": ["content", "author_profile_url"],
    "scope": "org"
  },
  "batching": {
    "strategy": "sequential",
    "batch_size": 20,
    "inter_batch_delay_ms": 2000
  },
  "output": {
    "format": "structured_dataset",
    "insights": ["lead_scoring", "trend_detection", "summary"]
  }
}
```

### Schema validation table

| Field | Type | Required | Validation rule |
|---|---|---|---|
| `schema_version` | string | yes | Must be `"1"` |
| `task_id` | string | yes | 16-char hex; idempotency key in scheduler |
| `intent` | string | yes | Must exist in `TaskRegistry` |
| `created_by.org_id` | int64 | yes | Injected server-side; never from LLM |
| `created_by.account_id` | int64 | yes | Injected server-side; never from LLM |
| `crawl_plan.sources` | array | yes | At least 1; all URLs must be `https://` and pass IP safety check |
| `crawl_plan.sources[].type` | string | yes | One of: `facebook_group`, `facebook_post`, `facebook_profile`, `web_url` |
| `sampling.max_total_items` | int | yes | `<= CRAWL_MAX_ITEMS_HARD_LIMIT` (default 1000) |
| `filters.language` | []string | no | Valid BCP-47 codes only |
| `filters.keyword_relevance_min` | float | no | `[0.0, 1.0]`; default 0.3 if omitted |
| `batching.strategy` | string | yes | `"sequential"` or `"parallel"` |
| `output.insights` | []string | no | Each element in `{lead_scoring, trend_detection, clustering, summary}` |

---

## Execution Flow (Strict)

### Normal path

```
[1] User sends text via Telegram / web chat / API
      orgID, accountID resolved from authenticated session

[2] POST /api/v1/tasks/parse
      AITaskParser.Parse(text, orgID, accountID)
      ├─ ErrAmbiguous → HTTP 200 { "clarification": "..." } — no task created
      ├─ ErrUnsupportedIntent → HTTP 422 { "error": "intent not supported" }
      └─ TaskJSON → proceed

[3] JobGenerator.Submit(taskJSON)
      → Validate schema
      → Validate intent in registry
      → store.CreateTask → tasks row (status=pending)
      → jobs.Submit(intent, task_id, taskJSON)
        → INSERT OR IGNORE scheduler_jobs
        → if duplicate: return existing task record
      → HTTP 202 { "task_id", "job_id", "status": "pending" }
      → Telegram: "Đang xử lý: <intent> — task_id: <id>"

[4] Scheduler worker claims job (async)
      → atomic UPDATE scheduler_jobs: pending → running
      → TaskRegistry.Get(intent) → handler
      → store.StartTask(task_id)
      → handler.Handle(ctx, job)

[5] Handler executes crawl
      → BrowserRuntimeManager.StartContainer
      → Fetch loop with per-item FilterEngine.Evaluate
      → DuplicateChecker per passing item
      → InsightPipeline (if requested)
      → store.CompleteTask(OutputDataset)
      → BrowserRuntimeManager.StopContainer

[6] Scheduler marks job completed
      scheduler_jobs.status = 'completed'

[7] Caller notification
      Telegram: "✅ Hoàn thành <intent>: N bài viết, M leads — task_id: <id>"
      or webhook POST if API caller
```

### Failure paths

```
Parse ambiguity:
  → ErrAmbiguous returned to user as clarification question
  → No task row, no job row. Zero cost beyond one LLM call.

Validation failure (bad URL, unsupported intent):
  → HTTP 422 or 400 with specific field error
  → No task row, no job row.

Browser start timeout:
  → store.FailTask(task_id, "browser start timeout after Xs")
  → scheduler retry policy: constant 60s, max 2 attempts
  → After final failure: tasks.status = 'failed'; Telegram error notification

Crawl navigation error (page not found, auth expired):
  → store.FailTask with error string
  → Retry policy applies (same as above)

Handler crash mid-crawl (process restart):
  → stale job recovery resets scheduler_jobs to 'pending'
  → new worker re-executes handler from beginning
  → tasks row reset to 'running'
  → dedup_hash prevents re-insertion of already-written records
  → OutputDataset rebuilt clean
```

### Idempotency matrix

| Scenario | Mechanism | Result |
|---|---|---|
| Same user message sent twice | `task_id` = deterministic hash → same key | `INSERT OR IGNORE` no-ops; one execution |
| Telegram message received twice (delivery dup) | Same `task_id` | One `tasks` row, one `scheduler_jobs` row |
| Handler crashes and restarts | Stale recovery → re-execute | Records re-fetched; DB dedup prevents duplicate writes |
| Two workers race to claim same job | SQLite exclusive write lock on subquery UPDATE | One winner, one zero-row result; loser polls next cycle |

---

## Decisions

### 1. GPT-4o structured output mode — not function calling

**Decision**: Use `response_format={type:"json_schema", schema:<TaskSchemaV1>}` instead of function-calling tool_calls.

**Why**: Structured output mode guarantees the response matches the declared JSON Schema exactly (no `tool_calls` array parsing, no conditional `finish_reason` checks). The parser always receives a valid `TaskJSON` or an OpenAI API error — no intermediate ambiguity state.

**Alternative considered**: Function-calling as used in the old `SkillRouter` — reuses familiar code but introduces a routing step where the model selects a "function" (skill), which is exactly the abstraction we are removing. Structured output produces a complete task document in one call.

### 2. `task_id` is a deterministic content hash — not a UUID

**Decision**: `task_id = hex(sha256(intent + sorted_keywords + truncate(input_text,200) + date_utc_day))[:16]`.

**Why**: A UUID generated at request time would allow duplicate tasks for identical user commands — the user sends the same Telegram message twice and gets two crawl jobs. The content hash ensures the same intent + same keywords + same day maps to the same `task_id`, hitting `INSERT OR IGNORE` on the second submission. The day truncation means the same command on a new day gets a fresh task.

**Alternative considered**: Store a hash of the full original text — too strict; minor rephrasing creates a new task even when the semantic intent is identical. Intent + keywords captures semantic equivalence without requiring exact text match.

### 3. Filter Engine is synchronous per-item — no batch post-filter

**Decision**: `FilterEngine.Evaluate(item, filters)` is called inline in the fetch loop. Items failing any filter are discarded immediately. No accumulation of raw items for later filtering.

**Why**: Post-collection filtering requires holding potentially thousands of raw items in memory before discarding most of them. In-crawl filtering discards items at the point of fetch, keeping the in-memory accumulator bounded to `max_total_items` (the count of passing items, not fetched items). It also allows early termination of the fetch loop as soon as `max_total_items` passing items are accumulated.

**Alternative considered**: Collect all items, then filter — simpler handler code but unbounded memory; rejects the fundamental design constraint.

### 4. Handlers stop their own containers — no ambient container lifecycle

**Decision**: Each handler calls `BrowserRuntimeManager.StartContainer` at the start and `StopContainer` at the end (even on error, via `defer`). The container is not left running for future tasks.

**Why**: Crawl tasks are long-running (30–120s). Leaving containers running between tasks wastes resources and port leases. The restart policy and warm pool handle startup latency mitigation; crawl handlers should not assume a persistent workspace.

**Alternative considered**: Keep containers warm across consecutive tasks for the same account — reduces startup overhead but complicates container lifecycle ownership (who is responsible for stopping it?). Keeping handlers fully responsible for their own containers is the simpler invariant.

### 5. Skill architecture removed entirely — no migration shim

**Decision**: Delete `internal/skills/` and `internal/ai/skill_router.go` without providing a compatibility wrapper.

**Why**: A shim that maps old skill names to new task intents would preserve the skill abstraction in the codebase even while it's removed from the spec. Any code that imports `internal/skills` after this change is a build error, which is the correct signal. Handlers are not skills; they do not share the `Skill` interface and should not be confused with it.

---

## Risks / Trade-offs

- **GPT-4o structured output is a newer API feature** → If the model returns an invalid JSON for a complex task, the API returns a hard error (not partial JSON). Mitigation: keep `TaskSchema` fields to a minimum; default optional fields to empty rather than requiring them in the JSON Schema.
- **Deterministic task_id means same-day duplicate commands do not retry failed tasks** → If a task fails and the user resends the same message the same day, `INSERT OR IGNORE` returns the failed row. Mitigation: `JobGenerator` checks the existing `tasks.status`; if `failed`, it deletes the old `scheduler_jobs` row (now terminal and purgeable) and submits a new job. This is the explicit retry path.
- **Handler owns container start/stop — cold-start adds 2–4s per task** → Mitigation: the warm pool (`warm-browser-pool` change) pre-warms containers. `BrowserRuntimeManager.StartContainer` hits the warm pool first; cold-start only on miss. Handlers do not need to know about the pool.
- **Filter Engine uses simple heuristics (n-gram language detection, TF-IDF keyword matching)** → For MVP, these are fast and have zero LLM cost. Mitigation: `InsightPipeline` runs after the clean dataset is assembled and can re-score or re-classify if higher accuracy is needed. The filter's job is to eliminate obvious noise cheaply; precise classification is an insight concern.

## Migration Plan

1. Delete `internal/skills/` and `internal/ai/skill_router.go`.
2. Drop and re-create `tasks` table with new schema (data loss of existing skill task history — acceptable, no production data yet).
3. Add `internal/ai/task_parser.go`, `internal/crawl/` package.
4. Deregister `skill_run` from job registry; register `facebook_crawl`, `lead_generation`, `visa_research`, `web_crawl`.
5. Update Telegram `OnText` handler to call `AITaskParser` + `JobGenerator`.
6. Update `POST /api/v1/tasks/parse` endpoint (was `TaskExecutor.Execute`, now `JobGenerator.Submit`).
7. Build and verify: `go build ./cmd/scraper/` must pass with no reference to `internal/skills`.
8. Rollback: restore `internal/skills/`, re-register `skill_run`, revert `tasks` table — all additive changes, no shared-state breakage.

## Open Questions

- Should `task_id` incorporate `account_id` in the hash? If yes, the same command from two different accounts produces two tasks (correct behavior). If no, account A's task blocks account B from running the same crawl. Proposed: **yes, include `account_id` in hash** — `sha256(account_id + intent + sorted_keywords + date_utc_day)[:16]`.
- Should `WebCrawlHandler` support login-required sites (using stored credentials)? Proposed: no for MVP — `web_crawl` is public-web only. Login-required web crawling goes through `facebook_crawl` which uses the authenticated browser workspace.
- Should `InsightPipeline` have a token budget cap per task? Proposed: yes — `INSIGHT_MAX_TOKENS=4000` env var; truncate the records dataset if needed before sending to GPT-4o.
