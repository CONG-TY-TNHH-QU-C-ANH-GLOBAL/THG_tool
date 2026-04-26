## ADDED Requirements

### Requirement: Fixed worker pool
The system SHALL start `JOB_WORKER_COUNT` (default 4) goroutines at scheduler startup. Each goroutine independently polls the job store, claims one job per iteration, executes it via the handler registry, and loops. Workers do not share state beyond the database connection.

#### Scenario: Workers start on scheduler start
- **WHEN** `Scheduler.Start()` is called
- **THEN** exactly `JOB_WORKER_COUNT` goroutines begin polling; the pool size does not change while the scheduler is running

#### Scenario: Workers stop on scheduler shutdown
- **WHEN** the scheduler's context is cancelled (application shutdown)
- **THEN** each worker finishes its current job (or wait-loop), then exits; `Scheduler.Stop()` returns only after all workers have exited

### Requirement: Poll-based job claim
Each worker SHALL attempt to claim a job by executing the atomic `UPDATE … RETURNING *` statement on every tick of `JOB_POLL_INTERVAL` (default 500ms). If no job is returned the worker sleeps for `JOB_POLL_INTERVAL` before the next attempt.

#### Scenario: Job claimed and executed
- **WHEN** the claim UPDATE returns a job row
- **THEN** the worker immediately dispatches the job to the registered handler without additional sleeping; the next poll begins after the handler returns

#### Scenario: No job available — worker sleeps
- **WHEN** the claim UPDATE returns zero rows
- **THEN** the worker sleeps for `JOB_POLL_INTERVAL` before the next claim attempt; no error is logged

### Requirement: Context propagation to handlers
The scheduler SHALL pass a per-job `context.Context` derived from the scheduler root context to `handler.Handle`. If the scheduler is shut down while a handler is running the context is cancelled, allowing the handler to abort in-flight work.

#### Scenario: Handler receives cancellation on shutdown
- **WHEN** the scheduler context is cancelled while `handler.Handle` is executing
- **THEN** the derived context passed to the handler is cancelled; if the handler returns early with a context error, the worker transitions the job back to `pending` (not `failed`) so it can be reclaimed after restart

#### Scenario: Handler context not shared across jobs
- **WHEN** two workers each claim a different job concurrently
- **THEN** each handler receives its own independent context; cancelling one does not affect the other

### Requirement: Worker pool size configuration
The system SHALL read `JOB_WORKER_COUNT` from the environment at startup. Values less than 1 SHALL be treated as 1.

#### Scenario: Default worker count
- **WHEN** `JOB_WORKER_COUNT` is not set in the environment
- **THEN** the scheduler starts 4 worker goroutines

#### Scenario: Custom worker count
- **WHEN** `JOB_WORKER_COUNT=8` is set in the environment
- **THEN** the scheduler starts exactly 8 worker goroutines
