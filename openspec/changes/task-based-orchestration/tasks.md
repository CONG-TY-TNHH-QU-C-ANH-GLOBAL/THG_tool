## 1. Remove Skill Architecture

- [ ] 1.1 Delete `internal/skills/` package entirely: `registry.go`, `executor.go`, `scrape_group.go`, `post_comment.go`, `send_inbox.go`, `check_notifications.go`, `get_profile_info.go`, `comment_hot_leads.go`, `chromedp_helpers.go`
- [ ] 1.2 Delete `internal/ai/skill_router.go`
- [ ] 1.3 Remove `skill_run` registration from `jobs.Registry` in `cmd/scraper/main.go`
- [ ] 1.4 Remove `SKILL_DEFAULT_ACCOUNT_ID`, `SKILL_MAX_STEPS`, `SKILL_ACTION_DELAY_MS` from `internal/config/config.go` and `.env.example`
- [ ] 1.5 Verify `go build ./cmd/scraper/` fails cleanly on removed imports (no lingering references to `internal/skills`)

## 2. Database Migration

- [ ] 2.1 Drop and recreate `tasks` table in `internal/store/store.go`: columns `id INTEGER PRIMARY KEY`, `task_id TEXT NOT NULL UNIQUE`, `org_id INTEGER NOT NULL DEFAULT 0`, `account_id INTEGER NOT NULL`, `intent TEXT NOT NULL`, `task_json TEXT NOT NULL`, `status TEXT NOT NULL DEFAULT 'pending'`, `result_json TEXT`, `error TEXT`, `crawl_steps_completed INTEGER NOT NULL DEFAULT 0`, `total_fetched INTEGER NOT NULL DEFAULT 0`, `total_returned INTEGER NOT NULL DEFAULT 0`, `parse_prompt_tokens INTEGER NOT NULL DEFAULT 0`, `parse_completion_tokens INTEGER NOT NULL DEFAULT 0`, `insight_prompt_tokens INTEGER NOT NULL DEFAULT 0`, `insight_completion_tokens INTEGER NOT NULL DEFAULT 0`, `duration_ms INTEGER NOT NULL DEFAULT 0`, `created_at DATETIME NOT NULL`, `updated_at DATETIME NOT NULL`
- [ ] 2.2 Add `idx_tasks_org_intent_status` index on `(org_id, intent, status, created_at DESC)`
- [ ] 2.3 Add `idx_tasks_task_id` unique index on `task_id` (if not covered by UNIQUE constraint)
- [ ] 2.4 Verify migration is idempotent; confirm existing `posts` and `leads` tables are untouched

## 3. Config Variables

- [ ] 3.1 Add `CRAWL_MAX_ITEMS_HARD_LIMIT` (int, default 1000) to `internal/config/config.go`
- [ ] 3.2 Add `CRAWL_BROWSER_START_TIMEOUT` (duration, default 30s)
- [ ] 3.3 Add `CRAWL_INTER_BATCH_DELAY_MS` (int, default 2000)
- [ ] 3.4 Add `CRAWL_TASK_MAX_ATTEMPTS` (int, default 2)
- [ ] 3.5 Add `CRAWL_INTENT_MATCH_THRESHOLD` (float64, default 0.2)
- [ ] 3.6 Add `CRAWL_SPAM_THRESHOLD` (float64, default 0.95 — uppercase ratio cap)
- [ ] 3.7 Add `LEAD_SCORE_THRESHOLD` (float64, default 0.7)
- [ ] 3.8 Add `INSIGHT_MAX_TOKENS` (int, default 4000)
- [ ] 3.9 Document all new vars in `.env.example`

## 4. AI Task Parser (`internal/ai/task_parser.go`)

- [ ] 4.1 Define `ParseTokenUsage` struct: `PromptTokens int`, `CompletionTokens int`
- [ ] 4.2 Define `ErrAmbiguous{Clarification string}`, `ErrUnsupportedIntent{Intent string}`, `ErrSchemaValidation{Field, Reason string}` error types
- [ ] 4.3 Define `TaskJSON` struct matching schema v1 fields (all nested structs: `CrawlPlan`, `Source`, `Query`, `TimeRange`, `Sampling`, `Filters`, `Engagement`, `DedupRules`, `Batching`, `Output`)
- [ ] 4.4 Implement `AITaskParser` struct: `client *openai.Client`, `model string`, `registry *crawl.TaskRegistry`
- [ ] 4.5 Implement `AITaskParser.Parse(ctx, text, orgID, accountID int64) (*TaskJSON, ParseTokenUsage, error)`: build `response_format` JSON Schema from `TaskSchema v1`; call `CreateChatCompletion`; unmarshal; inject `created_by.org_id` and `created_by.account_id` from params (overwrite any LLM value); compute `task_id` hash; validate `intent` in registry; return
- [ ] 4.6 Implement `computeTaskID(accountID int64, intent string, keywords []string) string`: `hex(sha256(itoa(accountID) + intent + strings.Join(sorted(keywords)," ") + date_utc_day()))[:16]`
- [ ] 4.7 Write unit tests: successful parse produces correct intent and task_id; `ErrAmbiguous` on empty intent; `ErrUnsupportedIntent` on unknown intent; `created_by` always injected server-side; identical input same day → identical task_id; different day → different task_id

## 5. Task Registry (`internal/crawl/registry.go`)

- [ ] 5.1 Define `TaskHandler` interface: `Handle(ctx context.Context, job jobs.Job) error`
- [ ] 5.2 Implement `TaskRegistry` struct: `entries map[string]TaskHandler` (set at startup)
- [ ] 5.3 Implement `Register(intent string, h TaskHandler)`, `Get(intent string) (TaskHandler, bool)`, `All() []string`
- [ ] 5.4 Confirm zero imports from `internal/` in this file (registry is a pure map)

## 6. Job Generator (`internal/crawl/job_generator.go`)

- [ ] 6.1 Implement `JobGenerator` struct: `registry *TaskRegistry`, `store *store.Store`, `scheduler *jobs.Scheduler`, `cfg config.Config`
- [ ] 6.2 Implement schema validator: checks `schema_version`, `intent` in registry, all source URLs (https, no private IPs), `max_total_items <= CRAWL_MAX_ITEMS_HARD_LIMIT`, BCP-47 language codes, source-type / intent compatibility
- [ ] 6.3 Implement `JobGenerator.Submit(ctx, taskJSON *TaskJSON, parseUsage ParseTokenUsage) (*TaskRecord, error)`: validate → `store.CreateTask` → `jobs.Submit(intent, task_id, payload)` → on `jobs.Submit` failure delete `tasks` row (compensating write) → return `TaskRecord`
- [ ] 6.4 Handle duplicate task_id: if existing `tasks.status = "failed"`, delete the terminal `scheduler_jobs` row and reset `tasks.status = "pending"` before re-submitting
- [ ] 6.5 Write unit tests: valid task submitted creates one `tasks` row and one `scheduler_jobs` row; duplicate task_id returns existing record; failed-task retry re-submits; bad URL rejected before any DB write; wrong intent rejected; over-limit sampling rejected

## 7. Filter Engine (`internal/crawl/filter/`)

- [ ] 7.1 Define `RawItem` struct (common base): `Content string`, `AuthorName string`, `AuthorProfileURL string`, `SourceURL string`, `Timestamp time.Time`, `Reactions int`, `Comments int`, `Shares int`
- [ ] 7.2 Define `FilterResult` struct: `Pass bool`, `Score float64`, `Signals []string`
- [ ] 7.3 Implement `FilterEngine.Evaluate(item RawItem, filters TaskFilters) FilterResult`: run 6-stage pipeline in order; short-circuit on first FAIL
- [ ] 7.4 Implement `KeywordRelevance` stage: Unicode-normalized case-insensitive token match; score = matched/total; FAIL if score < `filters.KeywordRelevanceMin`
- [ ] 7.5 Implement `SpamDetector` stage: substring match against `filters.Exclude`; length < 20 FAIL; uppercase ratio > `CRAWL_SPAM_THRESHOLD` FAIL
- [ ] 7.6 Implement `EngagementGate` stage: compare against `filters.Engagement` minimums (skip if all zeros)
- [ ] 7.7 Implement `LanguageFilter` stage: n-gram character frequency heuristic (Vietnamese: high diacritic density; English: Latin without diacritics); skip if `filters.Language` is empty
- [ ] 7.8 Implement `IntentMatchScorer` stage: TF-IDF overlap between item content tokens and `crawl_plan.query.keywords`; FAIL if score < `CRAWL_INTENT_MATCH_THRESHOLD`
- [ ] 7.9 Implement `QualityThreshold` stage: `AuthorProfileURL` empty → FAIL; `Timestamp` outside `time_range` → FAIL
- [ ] 7.10 Implement `DuplicateChecker.IsNew(item RawItem, orgID int64) bool`: `sha256(content+author_profile_url)` lookup against `posts.dedup_hash`
- [ ] 7.11 Write unit tests for each stage: keyword below threshold FAIL; spam match FAIL; all-caps FAIL; out-of-range timestamp FAIL; missing author FAIL; duplicate hash returns false; all-pass item returns `FilterResult{Pass:true}`

## 8. Facebook Crawl Handler (`internal/crawl/handlers/facebook.go`)

- [ ] 8.1 Implement `FacebookCrawlHandler` embedding `CrawlBase`: `Handle(ctx context.Context, job jobs.Job) error`
- [ ] 8.2 In `Handle`: `store.StartTask`; `BrowserRuntimeManager.StartContainer` with timeout poll; `defer BrowserRuntimeManager.StopContainer`; iterate sources; run fetch loop with `FilterEngine.Evaluate` + `DuplicateChecker.IsNew` per item; stop on `max_total_items`; `InsightPipeline.Run`; `store.CompleteTask`
- [ ] 8.3 Implement `fetchBatch(ctx, cdpSession, source Source, offset int, batchSize int) ([]RawItem, error)`: navigate to source URL + scroll/paginate; extract raw post/profile data using chromedp selectors; return slice
- [ ] 8.4 Implement `toStructuredRecord(item RawItem, signals []string) Record`: map RawItem fields to the `facebook_crawl` record schema including `filter_signals`
- [ ] 8.5 Write unit tests: fetch loop stops at `max_total_items`; filter applied per-item (not post-collection); delay sleep called between batches; deferred stop called even on error

## 9. Lead Gen Handler (`internal/crawl/handlers/leadgen.go`)

- [ ] 9.1 Implement `LeadGenHandler` embedding `FacebookCrawlHandler`: override `Handle` to add mandatory `lead_scoring` insight and `leads` table writes
- [ ] 9.2 After `InsightPipeline.Run`: iterate `lead_scores`; for each `score >= LEAD_SCORE_THRESHOLD`: check `SELECT 1 FROM leads WHERE source_url=? AND org_id=?`; if not exists: `store.InsertLead(orgID, accountID, taskID, record, score)`
- [ ] 9.3 Write unit tests: scores below threshold not written to leads; duplicate source_url not re-inserted; zero qualifying leads → no leads writes; `lead_scoring` insight always in output

## 10. Visa Research Handler (`internal/crawl/handlers/visa.go`)

- [ ] 10.1 Implement `VisaResearchHandler` embedding `CrawlBase`
- [ ] 10.2 Apply only `KeywordRelevance` and `QualityThreshold` filter stages
- [ ] 10.3 Implement structured extraction: `title` (h1/title tag), `deadline_date` (date pattern regex), `fee_amount` (currency pattern regex: VNĐ/USD/EUR with number), `document_list` (list items in main content), `contact_info` (phone/email patterns)
- [ ] 10.4 Mandate `summary` insight; append additional requested insights

## 11. Web Crawl Handler (`internal/crawl/handlers/web.go`)

- [ ] 11.1 Implement `WebCrawlHandler` embedding `CrawlBase`
- [ ] 11.2 Apply `KeywordRelevance`, `SpamDetector`, `IntentMatchScorer` filter stages only
- [ ] 11.3 Implement generic extraction: `title`, `description` (meta description or first paragraph), `price_signals` (regex: numbers + currency symbols/words), `contact_info` (phone/email/address patterns)

## 12. Insight Pipeline (`internal/ai/insight_pipeline.go`)

- [ ] 12.1 Define supported insight types: `lead_scoring`, `trend_detection`, `clustering`, `summary`
- [ ] 12.2 Implement `InsightPipeline.Run(ctx, records []Record, insights []string, orgLang string) (*InsightResult, InsightTokenUsage, error)`
- [ ] 12.3 Truncate records to `INSIGHT_MAX_TOKENS` budget before sending to GPT-4o
- [ ] 12.4 For `lead_scoring`: prompt returns `[{record_id, score, reason}]`
- [ ] 12.5 For `trend_detection`: prompt returns `[{keyword, frequency, rising}]`
- [ ] 12.6 For `clustering`: prompt returns `[{label, record_ids[]}]`
- [ ] 12.7 For `summary`: prompt returns Vietnamese summary string

## 13. Task Store (`internal/store/store.go` additions)

- [ ] 13.1 Implement `CreateTask(taskID, intent string, orgID, accountID int64, taskJSON string, parseUsage ParseTokenUsage) (*Task, error)`
- [ ] 13.2 Implement `StartTask(taskID string) error`: `UPDATE tasks SET status='running', updated_at=? WHERE task_id=?`
- [ ] 13.3 Implement `CompleteTask(taskID string, resultJSON string, stats TaskStats, insightUsage InsightTokenUsage, durationMs int64) error`
- [ ] 13.4 Implement `FailTask(taskID string, errMsg string, durationMs int64) error`
- [ ] 13.5 Implement `GetTaskByID(taskID string) (*Task, error)`
- [ ] 13.6 Implement `ListTasks(orgID int64, intent, status string, accountID int64, limit, offset int) ([]TaskSummary, int, error)`: `TaskSummary` omits `result_json` and `task_json`
- [ ] 13.7 Implement `InsertLead(orgID, accountID int64, sourceTaskID string, record Record, score float64) error`: used by `LeadGenHandler`

## 14. REST API Handlers

- [ ] 14.1 Update `POST /api/v1/tasks/parse` handler: call `AITaskParser.Parse` → `JobGenerator.Submit`; return `{task_id, job_id, status:"pending"}` on success; return clarification text on `ErrAmbiguous`; return 422 on `ErrUnsupportedIntent`
- [ ] 14.2 Update `GET /api/v1/tasks/:id` handler: call `store.GetTaskByID`; strip `total_fetched` and `task_json` from response; org ownership check (403 as 404)
- [ ] 14.3 Update `GET /api/v1/tasks` handler: use new `store.ListTasks` signature with `intent` filter; confirm `result` field is absent from list items
- [ ] 14.4 Verify both endpoints remain protected by `OrgScope` middleware

## 15. Telegram Bot Integration

- [ ] 15.1 Update `OnText` handler: call `AITaskParser.Parse` → on `ErrAmbiguous` reply with clarification → on success call `JobGenerator.Submit` → reply with "Đang xử lý: <intent> - ID: <task_id>"
- [ ] 15.2 Remove any remaining references to `SkillRouter.Route` or `TaskExecutor.Execute`
- [ ] 15.3 Implement completion callback: when handler writes `store.CompleteTask`, publish task_id to a notification channel; Telegram notifier goroutine reads and sends "✅ Hoàn thành <intent>: N kết quả — <task_id>"

## 16. Wiring in `main.go`

- [ ] 16.1 Instantiate `crawl.NewTaskRegistry()` and register all four handlers
- [ ] 16.2 Register all four job types in `jobs.Registry` with retry policies
- [ ] 16.3 Instantiate `ai.NewTaskParser(openaiClient, cfg.OpenAIModel, taskRegistry)`
- [ ] 16.4 Instantiate `crawl.NewJobGenerator(taskRegistry, store, jobScheduler, cfg)`
- [ ] 16.5 Confirm `internal/skills` has zero imports in the final build

## 17. Verification

- [ ] 17.1 `go build ./cmd/scraper/` passes with no reference to `internal/skills` or `SkillRouter`
- [ ] 17.2 `go test ./internal/crawl/... ./internal/ai/...` — all unit tests pass with `-race`
- [ ] 17.3 Manual: send Telegram "cào nhóm ship hàng mỹ <url>" → confirm `tasks` row created, `scheduler_jobs` row created, handler executes, `result_json` contains structured records
- [ ] 17.4 Manual: send same command twice → confirm second call returns same `task_id` from `GET /api/v1/tasks/:id`; only one `scheduler_jobs` row
- [ ] 17.5 Manual: confirm `GET /api/v1/tasks/:id` response does NOT contain `total_fetched` or `task_json`
- [ ] 17.6 Manual: send command for lead generation; confirm `leads` table receives rows with `score >= LEAD_SCORE_THRESHOLD`; confirm rows below threshold NOT in `leads`
- [ ] 17.7 Manual: send ambiguous Telegram message → confirm clarification reply returned, no task row created
- [ ] 17.8 Manual: attempt to access another org's task → confirm HTTP 403 returned as 404
