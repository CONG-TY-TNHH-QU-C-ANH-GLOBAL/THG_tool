## Context

The existing system has three disconnected pieces that this change unifies:

1. `internal/ai/` — an OpenAI client wrapper used for post classification, comment generation, and a partial skill router stub. The stub exists but is not wired to any execution path.
2. `internal/queue/` — a raw job queue that routes jobs to either the server Chrome process or a local agent. It has no concept of skills; callers specify raw job types.
3. `internal/browser/` — the Docker-based browser platform with a durable job scheduler (`scheduler_jobs`). It knows how to start containers but not what to do inside them once running.

Staff today either send ad-hoc Telegram messages that hit hard-coded command handlers, or use the web dashboard to manually trigger scrapers. There is no general-purpose "tell the system what to do in natural language" path, and no record of what was requested, executed, or produced beyond raw log output.

The `internal/workspace/workspace.go` package manages Chrome processes per account. Skill implementations use `chromedp` sessions attached to these workspaces. The connection point between "a skill needs a running Chrome for account X" and "the browser platform has started a container for account X" is the `workspace.Manager`, which already tracks live Chrome instances by `accountID`.

## Goals / Non-Goals

**Goals:**
- Single `SkillRouter` entry point: any text input → `{skillName, params}` or a clarification request.
- `SkillRegistry` as the canonical map of what the system can do; skill names are the public API.
- `TaskExecutor` owns the full execution lifecycle: account resolution → browser readiness → skill dispatch → result persistence → caller notification.
- `tasks` table provides a durable audit trail and the basis for the task history REST API.
- Six production skills implemented and registered at startup.
- Telegram bot commands and web chat both route through the same pipeline; no separate code paths.

**Non-Goals:**
- Multi-account fan-out (one command → run across all accounts). This is a future concern; skills target one account per invocation.
- Long-running skill sessions that span multiple HTTP requests or WebSocket frames. Skills are single-invocation, synchronous from the executor's perspective.
- Skill chaining / DAG execution (run skill A then skill B based on A's output). The `comment_hot_leads` skill handles the most common sequential case internally.
- Replacing the existing `internal/orchestrator` scan cycle. That runs on a timer; this change adds on-demand, command-driven execution alongside it.
- Image generation or image attachment in comments (user constraint: only use real uploaded images from `data/images/`).

## Decisions

### 1. GPT-4o function calling as the router — no custom NLP

**Decision**: `SkillRouter.Route(ctx, text)` calls the OpenAI Chat Completions API with a system prompt listing all registered skill schemas as JSON Schema function definitions. The model returns a `tool_calls` array. The router picks the first call, validates the parameters against the declared schema, and returns `(skillName, params, nil)`.

**Why**: Function calling is the most reliable LLM mechanism for structured extraction. The model produces valid JSON matching a declared schema. Hand-written regex or keyword matching breaks on Vietnamese input variation and would need updating every time a skill is added. Registering schemas dynamically (pulled from `SkillRegistry.All()` at call time) means adding a new skill automatically makes it routable — no router code changes.

**Alternative considered**: A fine-tuned classifier mapping Vietnamese phrases to skill names — more efficient but requires labelled training data and periodic retraining; rejected for MVP.

**Clarification flow**: If the model's `finish_reason` is `"stop"` (no tool call made), the router returns `(nil, nil, ErrNeedsMoreContext{Message: <model reply>})`. The caller presents the model's reply as a clarification question to the user.

### 2. `TaskExecutor` submits `skill_run` as a `scheduler_jobs` job type — async execution

**Decision**: `TaskExecutor.Execute(accountID, skillName, params)` calls `jobs.Submit("skill_run", idempotencyKey, payload)` and returns the job ID immediately. A registered `SkillRunHandler` in the job scheduler worker pool executes the skill asynchronously. Results are written to the `tasks` table by the handler; the caller polls `GET /api/v1/tasks/:id` or receives a Telegram reply via a callback channel.

**Why**: Skills like `scrape_group` may run for 30–120 seconds. Blocking the Telegram bot handler goroutine for that duration would prevent the bot from receiving other messages. Async dispatch via the durable job scheduler also survives process restarts — a scrape started before a redeploy completes after it.

**Alternative considered**: Goroutine-per-skill with a result channel — works within one process but loses in-flight tasks on restart and provides no history. Rejected.

**Idempotency key**: `skill_run:account:<id>:<skill>:<sha256(sorted params JSON)[:8]>`. The param hash means identical repeated commands (e.g., double-tap on Telegram) do not double-execute. Different params for the same skill on the same account produce different keys and are allowed to run concurrently.

### 3. `tasks` table is write-once from `SkillRunHandler`, read-many by the API

**Decision**: The `SkillRunHandler` creates a `tasks` row with `status='running'` before calling `skill.Run`, then updates it to `completed` or `failed` with the result JSON, token counts, and duration. The task history API reads this table directly. No in-memory cache.

**Why**: Keeps the `tasks` table consistent with the `scheduler_jobs` execution lifecycle. If the handler crashes mid-skill, the `tasks` row is left in `running` state; the stale job recovery path resets the `scheduler_jobs` row, which causes the handler to re-run and overwrite the stale `tasks` row. No orphaned task records.

**Alternative considered**: Writing results only to `scheduler_jobs.payload` — avoids a separate table but pollutes the generic scheduler with skill-domain fields (token counts, result JSON) and makes the history API dependent on the scheduler's internal schema.

### 4. Skills implement a single `Skill` interface; chromedp logic is self-contained per skill

**Decision**:
```go
type Skill interface {
    Name() string
    Description() string
    ParamSchema() map[string]any  // JSON Schema for OpenAI function calling
    Run(ctx context.Context, accountID int64, params map[string]any) (SkillResult, error)
}
```
Each skill file in `internal/skills/` implements this interface. `Run` receives a `context.Context` that is already attached to a live `chromedp.Session` for `accountID` via `workspace.Manager.Get(accountID)`. If the workspace is not running at call time, the executor starts a `browser_start` job and waits (with timeout) for the container to reach `running` state before dispatching.

**Why**: The interface is minimal. `ParamSchema()` returning a Go map avoids a separate YAML/JSON schema file per skill and keeps schema co-located with the implementation. The context-attached chromedp session pattern means skill code does not need to manage Chrome connections.

**Alternative considered**: Skills receive a `*chromedp.ExecAllocator` and create their own session — more flexible but each skill call opens and closes a Chrome session, discarding cookies and auth state. The workspace model's value is persistent session reuse; skill code must build on it, not bypass it.

### 5. Telegram bot gets a single command entry point — no per-skill command registration

**Decision**: The Telegram bot handles all non-command text messages with a single `OnText` handler that calls `SkillRouter.Route → TaskExecutor.Execute`. Explicit slash commands (`/start`, `/stop`) for browser control remain as-is. No per-skill slash commands are added.

**Why**: Skills are added and removed without touching the Telegram bot code. A user texting "cào group ship hàng mỹ" routes to `scrape_group` without any bot-side configuration. The bot's role is transport only; skill knowledge lives in the registry.

**Alternative considered**: Register each skill as a Telegram bot command (`/scrape_group`, `/post_comment`) — explicit but requires re-registering commands with Telegram's BotFather on every skill change; rejected.

## Risks / Trade-offs

- **GPT-4o latency adds ~1–2s to every Telegram command** → Mitigation: Router result is returned before execution begins; the user gets an acknowledgement message ("Starting skill...") immediately after routing, before the skill's 30–120s runtime.
- **`skill_run` jobs compete with `browser_start` jobs in the same worker pool** → Mitigation: Both use the same `scheduler_jobs` table and worker pool. At default `JOB_WORKER_COUNT=4`, a burst of skill submissions could delay browser starts. Mitigation: raise `JOB_WORKER_COUNT` or add a per-type worker pool partition in a follow-up change.
- **`scrape_group` and `comment_hot_leads` may hit Facebook rate limits** → Mitigation: Skills include configurable inter-action delays (`SKILL_ACTION_DELAY_MS`, default 1500ms). The selector healer (`internal/ai/selector_healer.go`) repairs broken CSS selectors without skill code changes.
- **Param hash in idempotency key prevents retrying a failed skill with same params** → Mitigation: Once a `skill_run` job reaches `failed`, the `tasks` row records the error. The retention purge deletes the `scheduler_jobs` row after `JOB_MAX_RETENTION` (24h), allowing resubmission. Users can also manually retry via the task history API (POST /api/v1/tasks/:id/retry — future).
- **Token cost accumulation** → Mitigation: `tasks` table stores `prompt_tokens` + `completion_tokens` per invocation. `GET /api/v1/tasks` aggregation gives total cost visibility. No hard cap in MVP; add `SKILL_MONTHLY_TOKEN_BUDGET` as a follow-up.

## Migration Plan

1. Add `tasks` table migration in `store.go`.
2. Implement `internal/skills/` package and register `SkillRunHandler` in `main.go`.
3. Implement `internal/ai/skill_router.go` (already has a stub — flesh out).
4. Update Telegram bot `OnText` handler to call `SkillRouter.Route`.
5. Add task history routes in `api.go`.
6. Deploy: existing in-flight scan cycles are unaffected; the new pipeline is purely additive.
7. Rollback: remove the `OnText` handler override; the bot reverts to its prior command-only behavior. `tasks` table and `scheduler_jobs` skill_run rows are harmless if left in place.

## Open Questions

- Should `TaskExecutor` wait for the browser container to be `running` before submitting the `skill_run` job, or submit both jobs and let the scheduler sequence them naturally? Proposed: submit `browser_start` first and wait (with a 30s timeout) for the `browser_containers` row to reach `running` before submitting `skill_run`. This avoids a `skill_run` job that claims a worker slot and then blocks waiting for a container.
- Should the `tasks` table be per-org queryable (filter by `org_id`)? Proposed: yes — add `org_id` column; `GET /api/v1/tasks` filters by caller's `orgID` via OrgScope middleware.
- Should `SkillResult` include structured data (e.g., a list of scraped posts) or only a summary string? Proposed: `SkillResult{Summary string, Data any}` where `Data` is JSON-serialized into `result_json`. Skills that produce records (scrape_group) return a post slice; skills that perform actions (post_comment) return a confirmation string.
