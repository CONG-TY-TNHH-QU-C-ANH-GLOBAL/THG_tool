## ADDED Requirements

### Requirement: Fixed worker pool drains the job queue
The system SHALL maintain a pool of goroutines (size = `SCHEDULER_WORKER_COUNT`) that continuously pull jobs from the FIFO queue and invoke `BrowserServicer.Start`. No goroutine SHALL be spawned per individual request.

#### Scenario: Worker picks up pending job
- **WHEN** a job enters the queue and a worker is idle
- **THEN** the worker transitions the job to `scheduled` and calls `BrowserServicer.Start` for the job's `account_id`

#### Scenario: All workers busy
- **WHEN** all workers are blocked waiting on the concurrency semaphore or executing `BrowserServicer.Start`
- **THEN** new jobs remain in the queue in `pending` state until a worker becomes available

#### Scenario: Default worker count
- **WHEN** `SCHEDULER_WORKER_COUNT` is not set in the environment
- **THEN** the worker pool size equals `MAX_CONCURRENT_BROWSERS`

### Requirement: Concurrency semaphore enforcement
The system SHALL use a semaphore (buffered channel of capacity `MAX_CONCURRENT_BROWSERS`) to ensure no more than `MAX_CONCURRENT_BROWSERS` containers are running at any time. A worker SHALL acquire a semaphore slot before calling `BrowserServicer.Start` and hold it until the container is stopped.

#### Scenario: Worker blocked at semaphore cap
- **WHEN** `MAX_CONCURRENT_BROWSERS` slots are all held and a worker picks up a new job
- **THEN** the worker blocks on the semaphore channel; the job remains in `scheduled` state until a slot is released

#### Scenario: Slot released on stop
- **WHEN** `POST /browser/stop` is called and the container is removed
- **THEN** exactly one semaphore slot is released, allowing the next blocked worker to proceed

#### Scenario: Slot released on start failure
- **WHEN** `BrowserServicer.Start` returns an error
- **THEN** the semaphore slot is released immediately and the job transitions to `failed`

### Requirement: Graceful scheduler shutdown
The system SHALL stop accepting new jobs and drain in-flight workers when the service receives a shutdown signal, without killing containers that are already running.

#### Scenario: Shutdown with pending jobs
- **WHEN** the service shuts down and jobs are still in the queue channel
- **THEN** pending jobs are transitioned to `failed` with reason `"service shutting down"` and no new containers are started

#### Scenario: Shutdown with in-flight starts
- **WHEN** the service shuts down while a worker is executing `BrowserServicer.Start`
- **THEN** the worker completes the current start (or cancels via context), transitions the job to `running` or `failed`, then exits cleanly

#### Scenario: Running containers survive shutdown
- **WHEN** the service process exits
- **THEN** Docker containers already in `running` state continue to run until explicitly stopped (Docker lifecycle is independent of the Go process)
