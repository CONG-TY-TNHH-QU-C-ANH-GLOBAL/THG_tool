## ADDED Requirements

### Requirement: Handler registration
The system SHALL provide a `Registry` that maps job type strings to `JobHandler` implementations and per-type retry policies. `Registry.Register(jobType string, handler JobHandler, policy RetryPolicy)` SHALL be called at application startup before the scheduler starts processing jobs. Registering the same type twice SHALL return an error.

#### Scenario: Successful registration
- **WHEN** `Registry.Register("browser_start", handler, policy)` is called at startup for a type not yet in the registry
- **THEN** the registry stores the handler and policy; subsequent job claims of type `"browser_start"` dispatch to that handler

#### Scenario: Duplicate registration rejected
- **WHEN** `Registry.Register` is called twice with the same `jobType` string
- **THEN** the second call returns a non-nil error; the first registration is unchanged

### Requirement: Handler interface
The system SHALL define a `JobHandler` interface with a single method `Handle(ctx context.Context, job Job) error`. Returning `nil` marks the job `completed`; returning a non-nil error marks it `failed` (or re-queues it if retries remain).

#### Scenario: Successful handler execution
- **WHEN** `handler.Handle(ctx, job)` returns `nil`
- **THEN** the worker transitions the job to `status='completed'` and updates `updated_at`

#### Scenario: Handler returns error with retries remaining
- **WHEN** `handler.Handle(ctx, job)` returns a non-nil error and `job.Attempt < job.MaxAttempts`
- **THEN** the worker increments `attempt`, sets `status='pending'`, computes `run_after` from the retry policy, and stores the error in `error` column; the job is retried on the next claim cycle

#### Scenario: Handler returns error with no retries left
- **WHEN** `handler.Handle(ctx, job)` returns a non-nil error and `job.Attempt >= job.MaxAttempts`
- **THEN** the worker transitions the job to `status='failed'` and stores the error string; no further claim attempts are made for this job

### Requirement: Per-type retry policy
The system SHALL associate a `RetryPolicy{MaxAttempts int, BackoffStrategy string, RetryDelay time.Duration}` with each registered job type. `BackoffStrategy` SHALL accept values `"constant"` (always `RetryDelay`) and `"exponential"` (`RetryDelay * 2^attempt`).

#### Scenario: Constant backoff delay
- **WHEN** a job of a type configured with `BackoffStrategy="constant"` and `RetryDelay=30s` fails on attempt 1
- **THEN** `run_after` is set to `now + 30s` for all retry attempts

#### Scenario: Exponential backoff delay
- **WHEN** a job of a type configured with `BackoffStrategy="exponential"` and `RetryDelay=10s` fails on attempt 2
- **THEN** `run_after` is set to `now + 40s` (10s * 2^2)

### Requirement: Unknown type rejection at submission
The system SHALL reject `Submit` calls for job types that have no registered handler, returning an error before inserting any row.

#### Scenario: Unregistered type rejected
- **WHEN** `jobs.Submit("unknown_type", key, payload)` is called and `"unknown_type"` is not in the registry
- **THEN** `Submit` returns a non-nil error and no row is inserted into `scheduler_jobs`
