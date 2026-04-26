## ADDED Requirements

### Requirement: Async task submission via job scheduler
The system SHALL implement `TaskExecutor.Execute(ctx context.Context, accountID int64, skillName string, params map[string]any) (*Task, error)` that validates the skill exists, validates params against the skill's schema, and calls `jobs.Submit("skill_run", idempotencyKey, payload)`. It SHALL return the `Task` record immediately without waiting for the skill to complete.

#### Scenario: Successful task submission
- **WHEN** `TaskExecutor.Execute(ctx, 42, "scrape_group", {"url":"https://fb.com/groups/abc"})` is called and the skill is registered
- **THEN** a `scheduler_jobs` row with `type="skill_run"` is inserted; a `tasks` row with `status="pending"` is created; the task record is returned to the caller with the `job_id`

#### Scenario: Unknown skill rejected before submission
- **WHEN** `TaskExecutor.Execute` is called with a `skillName` not in the registry
- **THEN** the executor returns an error; no `scheduler_jobs` row and no `tasks` row are created

#### Scenario: Idempotent submission — duplicate command not re-executed
- **WHEN** `TaskExecutor.Execute` is called twice with the same `accountID`, `skillName`, and `params` before the first completes
- **THEN** the second call returns the existing `Task` record; no second `scheduler_jobs` row is created; the skill runs exactly once

### Requirement: Browser readiness check before skill dispatch
The `SkillRunHandler` (job scheduler worker handling `skill_run` jobs) SHALL verify that the account's workspace is running before calling `skill.Run`. If the workspace is not running, the handler SHALL submit a `browser_start` job and wait up to 30 seconds for `browser_containers.state` to reach `running`. If the timeout elapses, the `skill_run` job SHALL be marked `failed`.

#### Scenario: Workspace already running — immediate dispatch
- **WHEN** the `SkillRunHandler` claims a `skill_run` job and `workspace.Manager.Get(accountID)` returns a live instance
- **THEN** the handler calls `skill.Run` immediately without submitting a `browser_start` job

#### Scenario: Workspace not running — start then dispatch
- **WHEN** the `SkillRunHandler` claims a `skill_run` job and no workspace is running for `accountID`
- **THEN** the handler calls `jobs.Submit("browser_start", ...)`, polls `browser_containers` every 2 seconds until `state="running"`, then calls `skill.Run`

#### Scenario: Browser start timeout
- **WHEN** the handler waits 30 seconds for `browser_containers.state="running"` and the state never arrives
- **THEN** the `skill_run` job is transitioned to `failed` with error `"browser start timeout"`; the `tasks` row is updated accordingly; retry policy applies normally

### Requirement: Task record lifecycle
The `SkillRunHandler` SHALL write to the `tasks` table at these points: (1) on job claim: insert row with `status="running"`, `created_at`; (2) on skill success: update `status="completed"`, `result_json`, `summary`, `prompt_tokens`, `completion_tokens`, `duration_ms`; (3) on skill failure: update `status="failed"`, `error`, `duration_ms`.

#### Scenario: Task record written on claim
- **WHEN** the `SkillRunHandler` claims a `skill_run` job
- **THEN** a `tasks` row with `status="running"` and the job's `account_id`, `skill`, `params_json` is inserted before `skill.Run` is called

#### Scenario: Task record updated on completion
- **WHEN** `skill.Run` returns `(SkillResult, nil)`
- **THEN** the `tasks` row is updated: `status="completed"`, `result_json=JSON(SkillResult.Data)`, `summary=SkillResult.Summary`, `duration_ms` set to elapsed time

#### Scenario: Task record updated on failure
- **WHEN** `skill.Run` returns a non-nil error
- **THEN** the `tasks` row is updated: `status="failed"`, `error=err.Error()`, `duration_ms` set; if retries remain the `scheduler_jobs` row is reset to `pending` but the `tasks` row stays as `failed` (a new `tasks` row is created on the retry attempt)

### Requirement: Caller notification on completion
The system SHALL send a Telegram reply to the originating chat when a task completes or fails, using the `Summary` field or error message respectively. The task payload SHALL carry the `chatID` so the `SkillRunHandler` can dispatch the reply without a separate lookup.

#### Scenario: Telegram reply on success
- **WHEN** a `skill_run` job submitted from a Telegram message completes
- **THEN** the bot sends `SkillResult.Summary` to the originating `chatID` stored in the job payload

#### Scenario: Telegram reply on failure
- **WHEN** a `skill_run` job fails after exhausting retries
- **THEN** the bot sends an error message including the skill name and error string to the originating `chatID`
