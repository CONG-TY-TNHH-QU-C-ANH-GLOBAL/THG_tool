## 1. Database Migration

- [ ] 1.1 Add `tasks` table migration in `internal/store/store.go`: columns `id INTEGER PRIMARY KEY`, `account_id INTEGER NOT NULL`, `org_id INTEGER NOT NULL DEFAULT 0`, `skill TEXT NOT NULL`, `params_json TEXT NOT NULL DEFAULT '{}'`, `status TEXT NOT NULL DEFAULT 'pending'`, `summary TEXT`, `result_json TEXT`, `error TEXT`, `prompt_tokens INTEGER NOT NULL DEFAULT 0`, `completion_tokens INTEGER NOT NULL DEFAULT 0`, `duration_ms INTEGER NOT NULL DEFAULT 0`, `created_at DATETIME NOT NULL`, `updated_at DATETIME NOT NULL`
- [ ] 1.2 Add index `idx_tasks_org_skill_status` on `(org_id, skill, status, created_at)` for list query performance
- [ ] 1.3 Verify migration is idempotent (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`)

## 2. Config Variables

- [ ] 2.1 Add `SKILL_DEFAULT_ACCOUNT_ID` (int64, default 0 = no default) to `internal/config/config.go`; used when a Telegram command does not specify an account
- [ ] 2.2 Add `SKILL_MAX_STEPS` (int, default 50) to `internal/config/config.go`
- [ ] 2.3 Add `SKILL_ACTION_DELAY_MS` (int, default 1500) to `internal/config/config.go`
- [ ] 2.4 Add all new vars to `.env.example` with Vietnamese documentation comments

## 3. Skill Interface and Registry (`internal/skills/registry.go`)

- [ ] 3.1 Define `SkillResult` struct: `Summary string`, `Data any`
- [ ] 3.2 Define `Skill` interface: `Name() string`, `Description() string`, `ParamSchema() map[string]any`, `Run(ctx context.Context, accountID int64, params map[string]any) (SkillResult, error)`
- [ ] 3.3 Implement `SkillRegistry` struct with `mu sync.RWMutex` and `skills map[string]Skill`
- [ ] 3.4 Implement `SkillRegistry.Register(skill Skill) error`: return error on duplicate name
- [ ] 3.5 Implement `SkillRegistry.Get(name string) (Skill, bool)`
- [ ] 3.6 Implement `SkillRegistry.All() []Skill` (returns snapshot slice for safe concurrent reads after startup)
- [ ] 3.7 Implement param schema validation helper `validateParams(schema map[string]any, params map[string]any) error`: check required fields present, return descriptive error for missing ones

## 4. AI Skill Router (`internal/ai/skill_router.go`)

- [ ] 4.1 Define `RouteResult` struct: `SkillName string`, `Params map[string]any`, `PromptTokens int`, `CompletionTokens int`
- [ ] 4.2 Define `ErrNeedsMoreContext` struct: `Message string`; implement `error` interface
- [ ] 4.3 Define `ErrUnroutableIntent` sentinel error
- [ ] 4.4 Implement `SkillRouter` struct: `client *openai.Client`, `model string`, `registry *skills.SkillRegistry`
- [ ] 4.5 Implement `SkillRouter.Route(ctx context.Context, text string) (*RouteResult, error)`: build function definitions from `registry.All()`; call `client.CreateChatCompletion`; parse `tool_calls`; return `ErrNeedsMoreContext` on stop finish reason, `ErrUnroutableIntent` if model returns no tool call and no informative reply
- [ ] 4.6 Write unit tests: mock OpenAI response with tool_call → correct skill extracted; mock stop response → ErrNeedsMoreContext; schema includes all registered skills

## 5. Task Store (`internal/store/store.go` additions)

- [ ] 5.1 Implement `CreateTask(accountID, orgID int64, skill, paramsJSON string) (*Task, error)`: insert with `status='pending'`
- [ ] 5.2 Implement `StartTask(taskID int64) error`: `UPDATE tasks SET status='running', updated_at=? WHERE id=?`
- [ ] 5.3 Implement `CompleteTask(taskID int64, summary, resultJSON string, promptTokens, completionTokens, durationMs int64) error`
- [ ] 5.4 Implement `FailTask(taskID int64, errMsg string, durationMs int64) error`
- [ ] 5.5 Implement `GetTask(taskID int64) (*Task, error)`
- [ ] 5.6 Implement `ListTasks(orgID int64, accountID int64, skill, status string, limit, offset int) ([]TaskSummary, int, error)`: `TaskSummary` omits `result_json`; `orgID=0` returns all orgs (superadmin)

## 6. Task Executor (`internal/skills/executor.go`)

- [ ] 6.1 Implement `TaskExecutor` struct: `registry *SkillRegistry`, `store *store.Store`, `jobs *jobs.Scheduler`, `workspace *workspace.Manager`, `cfg config.Config`
- [ ] 6.2 Implement `TaskExecutor.Execute(ctx context.Context, accountID, orgID int64, skillName string, params map[string]any, chatID int64) (*store.Task, error)`: validate skill exists; validate params; marshal params to JSON; compute idempotency key (`skill_run:account:<id>:<skill>:<sha256[:8]>`); call `store.CreateTask`; call `jobs.Submit("skill_run", key, payload)`; return task record
- [ ] 6.3 Implement `SkillRunHandler` struct satisfying `jobs.JobHandler`: `registry *SkillRegistry`, `store *store.Store`, `workspace *workspace.Manager`, `jobs *jobs.Scheduler`, `bot TelegramNotifier`
- [ ] 6.4 In `SkillRunHandler.Handle(ctx, job)`: unmarshal payload; `store.StartTask`; check workspace via `workspace.Manager.Get(accountID)`; if nil: submit `browser_start` job and poll `browser_containers` every 2s up to 30s; call `skill.Run`; on success: `store.CompleteTask`; on error: `store.FailTask`; send Telegram reply in both cases
- [ ] 6.5 Implement browser readiness poll helper `waitForBrowserRunning(ctx context.Context, accountID int64, store *store.Store, timeout time.Duration) error`: poll `store.GetBrowserContainer` every 2s; return nil when `state="running"`; return error on timeout

## 7. Skill Implementations (`internal/skills/`)

- [ ] 7.1 Create `scrape_group.go`: navigate to group URL, scroll and collect posts up to `max_posts`, dedup against `posts` table by `dedup_hash`, write new posts, return `[]Post`
- [ ] 7.2 Create `post_comment.go`: navigate to post URL, type message in comment box, optionally attach image from `data/images/` (validate path is under `data/images/`), submit; return summary
- [ ] 7.3 Create `send_inbox.go`: navigate to profile URL, click Message button, type and send message; return summary
- [ ] 7.4 Create `check_notifications.go`: open notifications panel, collect up to `max_items` notifications with text/url/timestamp; return `[]Notification`
- [ ] 7.5 Create `get_profile_info.go`: navigate to profile URL, extract name/bio/location/work/follower_count; return `ProfileInfo`
- [ ] 7.6 Create `comment_hot_leads.go`: query `leads` table for `score="hot" AND status="pending_comment"` filtered by org; iterate up to `max_comments`; call `internal/ai` comment generator for each; call `post_comment.Run`; update `leads.status="commented"`; return summary
- [ ] 7.7 Implement selector healing fallback in a shared `internal/skills/chromedp_helpers.go`: `ClickWithHeal(ctx, action, platform, selector string) error` wraps `chromedp.Click` with one SelectorHealer retry on failure; `TypeWithHeal`, `WaitVisibleWithHeal` follow the same pattern
- [ ] 7.8 Implement inter-action delay helper `ActionDelay(ctx context.Context, cfg config.Config)`: `time.Sleep(cfg.SkillActionDelayMs * time.Millisecond)`; call between consecutive browser interactions in all skills

## 8. REST API Handlers (`internal/server/`)

- [ ] 8.1 Create `internal/server/task_handlers.go` with `TaskHandlers` struct holding `store` reference
- [ ] 8.2 Implement `GET /api/v1/tasks/:id` handler: `store.GetTask`; org ownership check (return 403 as 404); return 200 or 404
- [ ] 8.3 Implement `GET /api/v1/tasks` handler: parse `account_id`, `skill`, `status`, `limit` (cap 200), `offset` query params; call `store.ListTasks(orgID, ...)`; return `{"tasks":[...],"total":N,"limit":N,"offset":N}`
- [ ] 8.4 Register both routes in `internal/server/api.go` under `/api/v1/tasks/` with `OrgScope` middleware

## 9. Telegram Bot Integration

- [ ] 9.1 Add `OnText` handler in the Telegram bot: receives any non-command message; resolves `accountID` (from message metadata or `SKILL_DEFAULT_ACCOUNT_ID`); calls `SkillRouter.Route`
- [ ] 9.2 On `ErrNeedsMoreContext`: send the clarification message back to the chat
- [ ] 9.3 On `ErrUnroutableIntent`: send "Xin lỗi, tôi không hiểu yêu cầu này" reply
- [ ] 9.4 On successful route: send acknowledgement "Đang thực hiện: <skill>..." then call `TaskExecutor.Execute`; reply with job ID
- [ ] 9.5 Implement `TelegramNotifier` interface (single method `Send(chatID int64, text string) error`) backed by the existing `gopkg.in/telebot.v3` bot; inject into `SkillRunHandler` for completion callbacks

## 10. Wiring in `main.go`

- [ ] 10.1 Instantiate `skills.NewRegistry()` and register all six skill implementations
- [ ] 10.2 Instantiate `ai.NewSkillRouter(openaiClient, cfg.OpenAIModel, skillRegistry)`
- [ ] 10.3 Instantiate `skills.NewTaskExecutor(skillRegistry, store, jobScheduler, workspaceMgr, cfg)`
- [ ] 10.4 Register `SkillRunHandler` in `jobRegistry.Register("skill_run", handler, jobs.RetryPolicy{MaxAttempts: 2, BackoffStrategy: "constant", RetryDelay: 30 * time.Second})`
- [ ] 10.5 Wire Telegram bot `OnText` to `SkillRouter` + `TaskExecutor`
- [ ] 10.6 Pass `TaskHandlers` to `server.New` for route registration

## 11. Verification

- [ ] 11.1 `go build ./cmd/scraper/` passes with no new warnings
- [ ] 11.2 `go test ./internal/skills/... ./internal/ai/...` — unit tests pass with `-race`: registry duplicate rejection, param schema validation, SkillRouter mock test, waitForBrowserRunning timeout, comment path outside data/images rejected
- [ ] 11.3 Manual test: send Telegram message "cào group ship hàng mỹ <url>" — confirm `GET /api/v1/tasks` shows a `scrape_group` task and a Telegram reply arrives
- [ ] 11.4 Manual test: send ambiguous message — confirm clarification reply from model is returned to Telegram chat
- [ ] 11.5 Manual test: send same command twice in 2 seconds — confirm only one `tasks` row created (idempotency)
- [ ] 11.6 Manual test: call `GET /api/v1/tasks?skill=scrape_group&status=completed` — confirm pagination and org scoping work
- [ ] 11.7 Manual test: send `post_comment` with `image_path=/etc/passwd` — confirm 400 error and no comment posted
