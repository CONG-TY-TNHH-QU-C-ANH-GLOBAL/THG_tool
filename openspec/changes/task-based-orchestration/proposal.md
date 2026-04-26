## Why

The system has accumulated two overlapping execution paths: the browser runtime manager handles container lifecycle correctly, but the layer above it — the skill-based `SkillRouter`/`SkillRegistry` abstraction — conflates UI automation primitives (click, type, selector) with business-level task intent (crawl leads, research visa services, find POD customers). The result is an architecture where adding a new crawl domain requires writing a new skill, registering it with the router, and extending the LLM prompt — three separate changes that must stay synchronized. Intent-based tasks (facebook lead crawl, POD customer discovery, visa service search) are fundamentally different from UI automation steps and must not share the same abstraction layer.

The entire skill-based layer is removed. In its place: a stateless AI Task Parser that converts free text into a versioned, validated Task JSON, a Job Generator that submits idempotent scheduler jobs, and a small set of domain-specific execution handlers that own all crawl logic, in-crawl filtering, deduplication, and structured output. The browser runtime manager and job scheduler are consumed unchanged.

## What Changes

- **REMOVED**: `internal/skills/` package entirely — `SkillRegistry`, `SkillRouter`, `SkillResult`, `TaskExecutor`, all six skill implementations, `skill_run` job type, `chromedp_helpers.go` selector wrappers.
- **REMOVED**: `internal/ai/skill_router.go` and all OpenAI function-calling routing code.
- **REMOVED**: `tasks` table columns that reference skill-domain concepts (`skill`, `params_json`, `summary`). Table is re-migrated with task-domain schema.
- **INTRODUCED**: `internal/ai/task_parser.go` — stateless LLM parser. Input: free text. Output: `TaskJSON` (schema v1). No side effects.
- **INTRODUCED**: `internal/crawl/` package — `TaskRegistry`, `JobGenerator`, `FilterEngine`, and one handler per intent (`FacebookCrawlHandler`, `LeadGenHandler`, `VisaResearchHandler`, `WebCrawlHandler`).
- **INTRODUCED**: `internal/crawl/filter/` — in-crawl filter pipeline (keyword relevance, spam detection, dedup, intent match, quality threshold). Applied per-item during fetch, never post-collection.
- **MODIFIED**: `tasks` table schema — replaces skill-oriented columns with task-oriented columns (`intent`, `task_json`, `result_json`).
- **MODIFIED**: Telegram bot `OnText` handler — routes through `AITaskParser` + `JobGenerator` instead of `SkillRouter` + `TaskExecutor`.
- **MODIFIED**: `browser-job-queue` — removes `skill_run` job type registration; adds `facebook_crawl`, `lead_generation`, `visa_research`, `web_crawl` job types.

## Capabilities

### New Capabilities

- `ai-task-parser`: Stateless LLM component that extracts intent, entities, keywords, crawl plan, and filters from free-text input. Returns a versioned `TaskJSON` or a typed error. No execution authority.
- `task-schema`: Versioned, validated JSON contract for all crawl tasks. Defines `task_id` (deterministic idempotency hash), `intent`, `crawl_plan` (step-based), `filters` (in-crawl rules), `batching`, `dedup_rules`, `output_schema`.
- `job-generator`: Converts `TaskJSON` → `scheduler_jobs` row via `jobs.Submit`. Validates intent against registry. Writes `tasks` row. Single submission point for all crawl work.
- `crawl-handler-facebook`: `FacebookCrawlHandler` — executes Facebook group/post/profile crawl plans with in-crawl filtering, dedup, batching, and structured output.
- `crawl-handler-leadgen`: `LeadGenHandler` — extends Facebook crawl with lead scoring insight pipeline and `leads` table writes.
- `crawl-handler-visa`: `VisaResearchHandler` — web-based research handler for visa/government/service page structured extraction.
- `crawl-handler-web`: `WebCrawlHandler` — general-purpose intent-based web crawl for POD/dropshipping discovery and other open-web intents.
- `crawl-filter-engine`: Per-item runtime filter pipeline. Components: keyword relevance scorer, spam detector, duplicate checker, intent match scorer, quality threshold gate. Rejected items are discarded without storage.
- `task-output-schema`: Structured `OutputDataset` contract — `records` array (filter-passing only), `stats` (fetch vs. returned counts), `insights`. No raw dumps.

### Modified Capabilities

- `browser-job-queue`: Removes `skill_run`. Adds `facebook_crawl`, `lead_generation`, `visa_research`, `web_crawl` job types with per-type retry policies.
- `task-history-api`: Adapts `GET /api/v1/tasks` and `GET /api/v1/tasks/:id` endpoints to the new task-domain schema columns.

## Impact

- **Code removed**: `internal/skills/` (entire package, ~8 files), `internal/ai/skill_router.go`.
- **Code added**: `internal/ai/task_parser.go`, `internal/crawl/` (registry, job_generator, filter/, handlers/facebook.go, handlers/leadgen.go, handlers/visa.go, handlers/web.go).
- **Database**: `tasks` table re-migrated (drop old columns, add `intent`, `task_json`, `crawl_steps_completed`, `total_fetched`, `total_returned`). Existing `posts` and `leads` tables unchanged.
- **APIs**: No breaking changes to external endpoints. `POST /api/v1/tasks/parse` replaces `SkillRouter` path. `GET /api/v1/tasks` and `GET /api/v1/tasks/:id` return updated field set.
- **Config**: New vars `CRAWL_MAX_ITEMS_HARD_LIMIT`, `CRAWL_BROWSER_START_TIMEOUT`, `LEAD_SCORE_THRESHOLD`, `CRAWL_SPAM_THRESHOLD`, `CRAWL_INTENT_MATCH_THRESHOLD`. Removed: `SKILL_DEFAULT_ACCOUNT_ID`, `SKILL_MAX_STEPS`, `SKILL_ACTION_DELAY_MS`.
- **Dependencies**: No new packages. Existing `go-openai`, `chromedp`, `modernc.org/sqlite`, Docker SDK.
