## ADDED Requirements

### Requirement: Enqueue browser start job
The system SHALL accept a browser start request and place it in a bounded FIFO queue, returning a job ID and queue position immediately without waiting for the container to start.

#### Scenario: Successful enqueue
- **WHEN** `POST /browser/start` is called with a valid `account_id` and the queue is not full
- **THEN** the system creates a job with state `pending`, enqueues it, and returns HTTP 202 with `{ "job_id", "status": "pending", "position": N, "account_id" }`

#### Scenario: Queue full — backpressure
- **WHEN** `POST /browser/start` is called and the number of pending jobs equals `MAX_QUEUE_DEPTH`
- **THEN** the system returns HTTP 429 with `{ "error": "queue full", "depth": MAX_QUEUE_DEPTH }` and does NOT create a job

#### Scenario: Queue depth default
- **WHEN** `MAX_QUEUE_DEPTH` is not set in the environment
- **THEN** the queue accepts up to 500 pending jobs before returning 429

### Requirement: Idempotent job submission
The system SHALL detect when a job for the same `account_id` already exists in a non-terminal state (`pending` or `running`) and return the existing job instead of creating a duplicate.

#### Scenario: Duplicate submission while pending
- **WHEN** `POST /browser/start` is called for an `account_id` that already has a job in state `pending`
- **THEN** the system returns HTTP 200 with the existing job's `{ "job_id", "status": "pending", "position": N }` and does not enqueue a new job

#### Scenario: Duplicate submission while running
- **WHEN** `POST /browser/start` is called for an `account_id` that already has a job in state `running`
- **THEN** the system returns HTTP 200 with `{ "job_id", "status": "running", "cdp_port", "vnc_port", "container_id" }` and does not enqueue a new job

#### Scenario: Resubmission after failure is allowed
- **WHEN** `POST /browser/start` is called for an `account_id` whose previous job reached state `failed`
- **THEN** the system creates a new job and enqueues it normally

### Requirement: Job state transitions
The system SHALL enforce the job state machine: `pending → scheduled → running → failed | completed`. No other transitions SHALL be permitted.

#### Scenario: Valid transition from pending
- **WHEN** a worker picks up a pending job
- **THEN** the job state transitions to `scheduled` before `BrowserServicer.Start` is called

#### Scenario: Valid transition to running
- **WHEN** `BrowserServicer.Start` returns successfully for a scheduled job
- **THEN** the job state transitions to `running` and the job record stores `cdp_port`, `vnc_port`, `container_id`

#### Scenario: Valid transition to failed
- **WHEN** `BrowserServicer.Start` returns an error for a scheduled job
- **THEN** the job state transitions to `failed` and the job record stores the error message

#### Scenario: Running job completed on stop
- **WHEN** `POST /browser/stop` is called for an account whose job is in state `running`
- **THEN** the job state transitions to `completed` after the container is stopped

### Requirement: Terminal job cleanup
The system SHALL remove job records from the idempotency index once they reach a terminal state (`failed` or `completed`), freeing memory and allowing future submissions for the same `account_id`.

#### Scenario: Failed job removed from index
- **WHEN** a job transitions to `failed`
- **THEN** the `account_id → job_id` idempotency index entry is removed within the same state transition

#### Scenario: Completed job removed from index
- **WHEN** a job transitions to `completed`
- **THEN** the `account_id → job_id` idempotency index entry is removed within the same state transition
