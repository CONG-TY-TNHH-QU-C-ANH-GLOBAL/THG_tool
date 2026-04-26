## Why

The browser automation platform can start, manage, and stream isolated Chrome containers per Facebook account, but has no layer that translates staff intent ("scrape hot leads from this group and comment on them") into the sequenced browser actions that execute it. Commands arrive via Telegram or the web UI as raw text; there is no structured skill dispatch, no multi-step task sequencing, and no durable record of what was attempted and what it produced.

## What Changes

- Introduce a `SkillRouter` that uses GPT-4o function-calling to map a natural language command to a named skill and its validated parameters.
- Introduce a `SkillRegistry` that maps skill names to `Skill` implementations with declared parameter schemas and required capabilities.
- Introduce a `TaskExecutor` that resolves the target account, acquires a running browser workspace via the existing scheduler, and calls `Skill.Run(ctx, accountID, params)`.
- Add a `tasks` SQLite table to record each task invocation — skill name, parameters, result, duration, token cost — as a durable audit trail.
- Wire Telegram bot commands and web chat input through the `SkillRouter` → `TaskExecutor` pipeline.
- Implement the first six production skills: `scrape_group`, `post_comment`, `send_inbox`, `check_notifications`, `get_profile_info`, `comment_hot_leads`.

## Capabilities

### New Capabilities

- `ai-skill-router`: GPT-4o function-calling router that maps free-text input to `{skill, params}`. Handles ambiguous input, missing required params (clarification prompt), and unknown intents (graceful rejection).
- `skill-registry`: Typed registry mapping skill name strings to `Skill` interface implementations with JSON Schema parameter validation. Skills declare required browser capabilities so the executor can verify account readiness.
- `task-executor`: Resolves account, checks browser availability (calls `jobs.Submit("browser_start", ...)` if needed), calls `skill.Run`, writes result to `tasks` table, reports back to caller.
- `skill-implementations`: The six named skills backed by `chromedp`: `scrape_group`, `post_comment`, `send_inbox`, `check_notifications`, `get_profile_info`, `comment_hot_leads`.
- `task-history-api`: `GET /api/v1/tasks` (paginated list by account/skill/status) and `GET /api/v1/tasks/:id` (full result + token cost).

### Modified Capabilities

- `browser-job-queue`: Skill execution tasks (`skill_run`) are submitted as a second job type alongside `browser_start`. The idempotency key is `skill_run:account:<id>:<skill>:<hash(params)>` so duplicate Telegram messages do not double-execute.

## Impact

- **Code**: New `internal/skills/` package (`registry.go`, `executor.go`, `router.go`, six skill files); new `internal/ai/skill_router.go`; new `tasks` table migration in `store.go`; new `GET /api/v1/tasks` routes; Telegram bot handler updated to pipe messages through `SkillRouter`.
- **APIs**: Two new read-only REST endpoints for task history. No breaking changes to existing browser, job, or auth endpoints.
- **Dependencies**: No new external packages — uses existing `github.com/sashabaranov/go-openai` client already wired in `internal/ai/`.
- **Config**: `OPENAI_MODEL` (already exists), `SKILL_DEFAULT_ACCOUNT_ID` (fallback account for Telegram commands that omit account context), `SKILL_MAX_STEPS` (max chromedp steps per skill invocation, default 50).
- **Database**: New `tasks` table (`id`, `account_id`, `skill`, `params_json`, `status`, `result_json`, `error`, `prompt_tokens`, `completion_tokens`, `duration_ms`, `created_at`, `updated_at`).
