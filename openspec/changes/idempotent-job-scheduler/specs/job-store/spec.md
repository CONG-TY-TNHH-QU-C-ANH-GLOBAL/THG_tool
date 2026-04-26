## ADDED Requirements

### Requirement: Persistent job table
The system SHALL maintain a `scheduler_jobs` SQLite table as the single source of truth for job state, with columns: `id`, `type`, `idempotency_key`, `payload` (JSON), `status`, `attempt`, `max_attempts`, `run_after`, `claimed_by`, `claimed_at`, `created_at`, `updated_at`, `error`. The table SHALL have a `UNIQUE(type, idempotency_key)` constraint and an index on `(status, run_after, created_at)`.

#### Scenario: Table created on startup
- **WHEN** the scheduler starts and the `scheduler_jobs` table does not exist
- **THEN** the migration in `store.go` creates the table with the UNIQUE constraint and the composite index before any job operations

#### Scenario: Concurrent write safety
- **WHEN** two goroutines simultaneously attempt to INSERT a job with the same `(type, idempotency_key)` pair
- **THEN** exactly one INSERT succeeds; the other is silently ignored by `INSERT OR IGNORE`; both callers return the same job row via the unconditional fetch-by-key that follows

### Requirement: Idempotent job submission
The system SHALL accept a `Submit(jobType, idempotencyKey string, payload any) (*Job, error)` call and guarantee that submitting the same `(type, idempotency_key)` pair while a prior job with that key is in a non-terminal state returns the existing row unchanged.

#### Scenario: First submission creates job
- **WHEN** `Submit("browser_start", "account:42", payload)` is called and no row with that key exists
- **THEN** `INSERT OR IGNORE` creates a new row with `status='pending'` and `Submit` returns the new job

#### Scenario: Duplicate submission returns existing job
- **WHEN** `Submit("browser_start", "account:42", payload)` is called and a row with that key already exists in state `pending` or `running`
- **THEN** `INSERT OR IGNORE` is a no-op; the unconditional `SELECT ... WHERE type=? AND idempotency_key=?` returns the existing row; `Submit` returns that row with no side effects

#### Scenario: Resubmission after terminal purge
- **WHEN** a job with key `("browser_start", "account:42")` previously reached `failed` or `completed` and was deleted by the purge goroutine, and `Submit` is called again with the same key
- **THEN** `INSERT OR IGNORE` creates a new row and `Submit` returns the new pending job

### Requirement: Atomic job claim
The system SHALL claim the next eligible job via a single `UPDATE scheduler_jobs SET status='running', claimed_by=?, claimed_at=? WHERE id=(SELECT id FROM scheduler_jobs WHERE status='pending' AND run_after <= ? ORDER BY created_at LIMIT 1) RETURNING *` statement. If no eligible job exists the statement returns zero rows.

#### Scenario: Single winner under concurrent claim
- **WHEN** four worker goroutines simultaneously execute the claim UPDATE for the same pending job row
- **THEN** exactly one goroutine receives the row in its `RETURNING *` result; the other three receive zero rows and proceed to sleep until the next poll interval

#### Scenario: No job available
- **WHEN** the claim UPDATE is executed and no row satisfies `status='pending' AND run_after <= now`
- **THEN** the statement returns zero rows; the worker sleeps for `JOB_POLL_INTERVAL` before retrying

#### Scenario: Deferred job not claimed early
- **WHEN** a job exists with `run_after` set 10 minutes in the future
- **THEN** the claim UPDATE skips that row and returns zero rows until the clock reaches `run_after`

### Requirement: Stale claim recovery
The system SHALL run a background goroutine that periodically resets jobs where `status='running' AND claimed_at < (now - JOB_CLAIMED_TIMEOUT)` back to `status='pending'` with `claimed_by=NULL` and `claimed_at=NULL`.

#### Scenario: Stale job reset after timeout
- **WHEN** a job has been in `status='running'` for longer than `JOB_CLAIMED_TIMEOUT` (default 5 minutes)
- **THEN** the recovery goroutine resets the job to `status='pending'`; a worker claims and re-executes it on the next poll cycle

#### Scenario: Live job not reset
- **WHEN** a job has been in `status='running'` for less than `JOB_CLAIMED_TIMEOUT`
- **THEN** the recovery goroutine does not touch the row

### Requirement: Terminal job retention and purge
The system SHALL keep jobs in `failed` or `completed` state for `JOB_MAX_RETENTION` (default 24h) to serve as an audit trail, then delete them. Deletion of a terminal row frees the `UNIQUE(type, idempotency_key)` slot for future resubmission.

#### Scenario: Terminal job retained within retention window
- **WHEN** a job transitions to `failed` or `completed` and `JOB_MAX_RETENTION` has not elapsed since `updated_at`
- **THEN** the job row remains in `scheduler_jobs` and is returned by status queries

#### Scenario: Terminal job purged after retention window
- **WHEN** a job's `updated_at` is older than `JOB_MAX_RETENTION`
- **THEN** the purge goroutine deletes the row; subsequent `Submit` with the same `(type, idempotency_key)` creates a new pending job
