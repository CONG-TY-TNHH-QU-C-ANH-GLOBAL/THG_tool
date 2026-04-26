## ADDED Requirements

### Requirement: `skill_run` job type registered alongside `browser_start`
The system SHALL support a second job type `"skill_run"` in the `scheduler_jobs` table. The `SkillRunHandler` SHALL be registered in `jobs.Registry` at startup with its own `RetryPolicy`. The payload for `skill_run` jobs SHALL be a JSON object containing `account_id`, `org_id`, `skill`, `params`, `task_id`, and `chat_id` (for Telegram reply routing).

#### Scenario: skill_run job submitted and claimed by worker
- **WHEN** `jobs.Submit("skill_run", "skill_run:account:42:scrape_group:abc123", payload)` is called
- **THEN** a `scheduler_jobs` row with `type="skill_run"` and `status="pending"` is inserted; a worker claims it via the standard atomic claim UPDATE on the next poll cycle

#### Scenario: skill_run and browser_start jobs share the same worker pool
- **WHEN** both `browser_start` and `skill_run` pending jobs exist simultaneously
- **THEN** workers claim whichever job has the earliest `created_at`; neither type has scheduling priority over the other

### Requirement: `skill_run` idempotency key scheme
The idempotency key for `skill_run` jobs SHALL be `skill_run:account:<id>:<skill>:<sha256(params_json)[:8]>`. This ensures the same command submitted twice (e.g., duplicate Telegram messages) maps to the same key and returns the existing job rather than creating a duplicate.

#### Scenario: Duplicate Telegram message does not double-execute
- **WHEN** the Telegram bot receives two identical messages within seconds and calls `jobs.Submit` twice with the same account, skill, and params
- **THEN** `INSERT OR IGNORE` on the second call is a no-op; only one `skill_run` job executes; the second `Execute` call returns the existing task record

#### Scenario: Same skill with different params creates a new job
- **WHEN** `jobs.Submit("skill_run", ...)` is called with the same account and skill but different `params`
- **THEN** the SHA-256 hash differs; a new `scheduler_jobs` row is created; both jobs may execute concurrently

### Requirement: `skill_run` retry policy
The `SkillRunHandler` SHALL be registered with `RetryPolicy{MaxAttempts: 2, BackoffStrategy: "constant", RetryDelay: 30s}`. Skills that fail due to transient browser errors (selector not found, navigation timeout) are retried; skills that fail due to validation errors (unknown skill, invalid params) SHALL not be retried and SHALL be marked `failed` immediately.

#### Scenario: Transient error retried
- **WHEN** `skill.Run` returns a `chromedp` navigation timeout error on attempt 1
- **THEN** the job is reset to `pending` with `run_after = now + 30s`; attempt 2 picks it up after the delay

#### Scenario: Validation error not retried
- **WHEN** the `SkillRunHandler` cannot find the skill name in the registry (registry lookup fails)
- **THEN** the job is transitioned directly to `failed` without incrementing `attempt` or scheduling a retry
