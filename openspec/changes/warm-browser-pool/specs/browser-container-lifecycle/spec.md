## MODIFIED Requirements

### Requirement: Start browser container for an account
The system SHALL check the warm pool before submitting a scheduler job. On a pool hit the system SHALL return container info synchronously (HTTP 200). On a pool miss the system SHALL fall through to the scheduler queue and return a job reference (HTTP 202). Idempotency, queue-depth limits, and concurrency cap enforcement remain as specified in the `browser-scheduler` change.

#### Scenario: Successful container start — pool hit
- **WHEN** `POST /browser/start` is called with a valid `account_id` and a warm slot exists for that account
- **THEN** the system claims the slot, returns HTTP 200 with `{ "status": "running", "cdp_port", "vnc_port", "container_id", "account_id" }`, and triggers background replenishment

#### Scenario: Successful container start submission — pool miss
- **WHEN** `POST /browser/start` is called with a valid `account_id` and no warm slot exists for that account and the queue is not full
- **THEN** the system enqueues a scheduler job and returns HTTP 202 with `{ "job_id", "status": "pending", "position": N, "account_id" }`

#### Scenario: Start when concurrency cap is reached but queue has space
- **WHEN** `POST /browser/start` is called and `MAX_CONCURRENT_BROWSERS` containers are already running but the queue is not full
- **THEN** the system returns HTTP 202 with `{ "job_id", "status": "pending", "position": N }` — the job waits in the queue until a slot is freed

#### Scenario: Start when container already running (idempotent)
- **WHEN** `POST /browser/start` is called for an account that already has a job in state `running`
- **THEN** the system returns HTTP 200 with `{ "job_id", "status": "running", "cdp_port", "vnc_port", "container_id" }` and does not claim a pool slot or enqueue a new job

#### Scenario: Profile directory created if absent
- **WHEN** the warm pool starts a container for an account whose profile directory does not exist
- **THEN** the system creates `data/profiles/account_{id}/` before starting the container
