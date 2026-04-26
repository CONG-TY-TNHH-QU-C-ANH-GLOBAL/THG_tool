## ADDED Requirements

### Requirement: Single submission point for all crawl tasks
The system SHALL implement `JobGenerator.Submit(ctx, taskJSON) (*TaskRecord, error)` as the only code path that calls `jobs.Submit` for crawl-type jobs. No handler, parser, or HTTP handler may call `jobs.Submit` for these job types directly.

#### Scenario: Successful task submission
- **WHEN** `JobGenerator.Submit` receives a valid `TaskJSON`
- **THEN** it writes a `tasks` row with `status="pending"` and calls `jobs.Submit(task.Intent, task.TaskID, marshaledPayload)`; returns `TaskRecord{task_id, job_id, status:"pending"}`

#### Scenario: Validation failure blocks all DB writes
- **WHEN** `JobGenerator.Submit` receives a `TaskJSON` that fails schema validation (e.g., private IP in source URL)
- **THEN** no `tasks` row is written and no `scheduler_jobs` row is written; a validation error is returned to the caller

### Requirement: Idempotent submission via task_id
Submitting a `TaskJSON` with a `task_id` that already exists in `scheduler_jobs` (for any status) SHALL return the existing `TaskRecord` without creating duplicate rows.

#### Scenario: Duplicate submission returns existing record
- **WHEN** `JobGenerator.Submit` is called twice with the same `task_id`
- **THEN** the second call's `jobs.Submit` executes `INSERT OR IGNORE` (no-op); the existing `tasks` row is fetched and returned; no duplicate `scheduler_jobs` row exists

#### Scenario: Failed task retry via explicit re-submit
- **WHEN** `JobGenerator.Submit` is called with a `task_id` whose `tasks.status = "failed"`
- **THEN** the generator deletes the terminal `scheduler_jobs` row (if present) and inserts a new one, resetting `tasks.status` to `"pending"`; this is the only permitted re-submit path

### Requirement: `tasks` row creation is atomic with job submission
The system SHALL write the `tasks` row before calling `jobs.Submit`. If `jobs.Submit` fails, the `tasks` row SHALL be deleted (compensating write). There SHALL never be a `tasks` row with `status="pending"` and no corresponding `scheduler_jobs` row.

#### Scenario: jobs.Submit failure rolls back tasks row
- **WHEN** `store.CreateTask` succeeds but `jobs.Submit` returns an error (e.g., scheduler not started)
- **THEN** the `tasks` row is deleted and the error is returned to the caller; no orphaned `tasks` row remains

### Requirement: Token usage recorded from parser
`JobGenerator.Submit` SHALL accept an optional `ParseTokenUsage` argument and store `parse_prompt_tokens` and `parse_completion_tokens` in the `tasks` row at creation time.

#### Scenario: Parse token cost persisted
- **WHEN** `AITaskParser.Parse` returns `ParseTokenUsage{PromptTokens:420, CompletionTokens:85}`
- **THEN** the `tasks` row is created with `parse_prompt_tokens=420` and `parse_completion_tokens=85`
