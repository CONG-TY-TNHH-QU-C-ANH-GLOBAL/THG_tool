## MODIFIED Requirements

### Requirement: Enqueue browser start job
The system SHALL accept a browser start request and place it in the durable SQLite-backed `scheduler_jobs` table via `jobs.Submit("browser_start", fmt.Sprintf("account:%d", accountID), payload)`, returning a job ID and status immediately without waiting for the container to start. The idempotency key `"account:<id>"` ensures duplicate submissions within the same non-terminal lifecycle return the existing job. The system SHALL verify `account_id` org ownership before submitting.

#### Scenario: Successful submission
- **WHEN** `POST /browser/start` is called with a valid `account_id` belonging to the caller's org
- **THEN** `jobs.Submit` inserts or retrieves a `scheduler_jobs` row with `status='pending'` and returns HTTP 202 with `{ "job_id", "status": "pending", "account_id" }`; no in-memory channel is used

#### Scenario: Duplicate submission while pending
- **WHEN** `POST /browser/start` is called for an `account_id` that already has a row in `scheduler_jobs` with `status='pending'`
- **THEN** `jobs.Submit` returns the existing row via `INSERT OR IGNORE` + unconditional fetch; HTTP 200 is returned with `{ "job_id", "status": "pending" }`; no second row is created

#### Scenario: Duplicate submission while running
- **WHEN** `POST /browser/start` is called for an `account_id` whose job is in `status='running'`
- **THEN** `jobs.Submit` returns the existing row; HTTP 200 is returned with `{ "job_id", "status": "running", "cdp_port", "vnc_port", "container_id" }`

#### Scenario: Resubmission after terminal purge
- **WHEN** `POST /browser/start` is called for an `account_id` whose previous job was deleted by the retention purge (reached `failed` or `completed` and exceeded `JOB_MAX_RETENTION`)
- **THEN** `jobs.Submit` creates a new `scheduler_jobs` row; HTTP 202 is returned with the new `job_id` and `status='pending'`

### Requirement: Job state transitions
The system SHALL enforce the job state machine via `scheduler_jobs.status`: `pending → running → failed | completed`. Workers claim jobs by transitioning `pending → running` atomically via the claim UPDATE. No other status values SHALL appear in `scheduler_jobs`.

#### Scenario: Worker claims pending job
- **WHEN** a worker goroutine executes the claim UPDATE and a pending browser_start job is available
- **THEN** the job status transitions to `running` in a single atomic UPDATE; the `browser_start` handler is invoked

#### Scenario: Handler success marks completed
- **WHEN** the `browser_start` handler returns `nil` (container started successfully)
- **THEN** the job status transitions to `completed`; `cdp_port`, `vnc_port`, and `container_id` are stored in the job payload

#### Scenario: Handler failure marks failed or retries
- **WHEN** the `browser_start` handler returns an error
- **THEN** if `attempt < max_attempts`, the job transitions back to `pending` with incremented `attempt` and computed `run_after`; if `attempt >= max_attempts`, the job transitions to `failed`

## REMOVED Requirements

### Requirement: In-memory job channel
**Reason**: Replaced by SQLite-backed `scheduler_jobs` table. The `chan *Job` FIFO channel and `sync.Map` job store in `internal/browser/` are deleted; all persistence and ordering is handled by `internal/jobs/`.
**Migration**: Call `jobs.Submit("browser_start", "account:<id>", payload)` instead of `JobQueue.Submit(accountID)`. Browser HTTP handlers import `internal/jobs` directly; `internal/browser/scheduler.go` and `job_queue.go` are deleted.

### Requirement: Queue full backpressure (HTTP 429)
**Reason**: The SQLite-backed scheduler has no fixed in-memory queue depth. Backpressure is now per-org concurrency caps enforced by `OrgSemaphoreRegistry` at the worker level, not at submission time. Submissions always succeed (idempotent insert); capacity limits are expressed at execution time.
**Migration**: Remove `MAX_QUEUE_DEPTH` env var. Per-org browser caps remain via `OrgSemaphoreRegistry`; workers block acquiring a semaphore slot, not the HTTP handler.
