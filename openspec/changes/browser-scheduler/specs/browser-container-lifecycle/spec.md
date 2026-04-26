## MODIFIED Requirements

### Requirement: Start browser container for an account
The system SHALL accept a browser start request and return a job reference immediately without waiting for the Docker container to start. The system SHALL enforce idempotency and queue-depth limits via the `browser-job-queue` capability; concurrency cap enforcement is delegated to the scheduler semaphore rather than direct rejection at the API layer.

#### Scenario: Successful container start submission
- **WHEN** `POST /browser/start` is called with a valid `account_id` and the queue is not full
- **THEN** the system returns HTTP 202 with `{ "job_id", "status": "pending", "position": N, "account_id" }` — no `cdp_port`, `vnc_port`, or `container_id` in this response

#### Scenario: Start when concurrency cap is reached but queue has space
- **WHEN** `POST /browser/start` is called and `MAX_CONCURRENT_BROWSERS` containers are already running but the queue is not full
- **THEN** the system returns HTTP 202 with `{ "job_id", "status": "pending", "position": N }` — the job waits in the queue until a slot is freed

#### Scenario: Start when container already running (idempotent)
- **WHEN** `POST /browser/start` is called for an account that already has a job in state `running`
- **THEN** the system returns HTTP 200 with `{ "job_id", "status": "running", "cdp_port", "vnc_port", "container_id" }` and does not enqueue a new job

#### Scenario: Profile directory created if absent
- **WHEN** the scheduler worker processes a job for an account whose profile directory does not exist
- **THEN** the system creates `data/profiles/account_{id}/` before calling `BrowserServicer.Start`
