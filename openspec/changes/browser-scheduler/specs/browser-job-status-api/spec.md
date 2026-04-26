## ADDED Requirements

### Requirement: Query individual job status
The system SHALL expose `GET /browser/jobs/:job_id` that returns the current state and result of a job, enabling clients to poll after a `POST /browser/start` submission.

#### Scenario: Pending job status
- **WHEN** `GET /browser/jobs/:job_id` is called for a job in state `pending`
- **THEN** the response is HTTP 200 with `{ "job_id", "account_id", "status": "pending", "position": N, "created_at" }`

#### Scenario: Scheduled job status
- **WHEN** `GET /browser/jobs/:job_id` is called for a job in state `scheduled`
- **THEN** the response is HTTP 200 with `{ "job_id", "account_id", "status": "scheduled", "created_at" }`

#### Scenario: Running job status
- **WHEN** `GET /browser/jobs/:job_id` is called for a job in state `running`
- **THEN** the response is HTTP 200 with `{ "job_id", "account_id", "status": "running", "cdp_port", "vnc_port", "container_id", "started_at" }`

#### Scenario: Failed job status
- **WHEN** `GET /browser/jobs/:job_id` is called for a job in state `failed`
- **THEN** the response is HTTP 200 with `{ "job_id", "account_id", "status": "failed", "error": "<message>", "failed_at" }`

#### Scenario: Unknown job ID
- **WHEN** `GET /browser/jobs/:job_id` is called with a job ID not in the store
- **THEN** the system returns HTTP 404 with `{ "error": "job not found" }`

### Requirement: Queue depth and running count visibility
The system SHALL expose `GET /browser/queue` that returns real-time queue depth and running container count for operational monitoring.

#### Scenario: Queue with pending and running jobs
- **WHEN** `GET /browser/queue` is called while jobs are pending and containers are running
- **THEN** the response is HTTP 200 with `{ "pending": N, "running": M, "capacity": MAX_CONCURRENT_BROWSERS, "queue_depth": MAX_QUEUE_DEPTH }`

#### Scenario: Empty queue
- **WHEN** `GET /browser/queue` is called with no jobs in queue and no running containers
- **THEN** the response is `{ "pending": 0, "running": 0, "capacity": MAX_CONCURRENT_BROWSERS, "queue_depth": MAX_QUEUE_DEPTH }`

### Requirement: Queue position tracking
The system SHALL report the position of a pending job in the queue (1 = next to be picked up) in both the `POST /browser/start` response and the job status response.

#### Scenario: Position decrements as jobs are processed
- **WHEN** a pending job at position 3 has the two jobs ahead of it picked up by workers
- **THEN** `GET /browser/jobs/:job_id` returns `"position": 1` for that job

#### Scenario: Position not present for non-pending jobs
- **WHEN** `GET /browser/jobs/:job_id` is called for a job in state `running`, `failed`, or `completed`
- **THEN** the response does NOT include a `position` field
