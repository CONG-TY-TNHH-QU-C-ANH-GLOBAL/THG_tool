## MODIFIED Requirements

### Requirement: Start browser container for an account
The system SHALL verify that the `account_id` in the request belongs to the caller's `org_id` before checking the warm pool or submitting a scheduler job. On org mismatch the system SHALL return HTTP 403. On a pool hit the system SHALL return container info synchronously (HTTP 200). On a pool miss the system SHALL fall through to the scheduler queue and return a job reference (HTTP 202). The concurrency cap enforced is the caller org's effective quota (`org.EffectiveQuota().MaxConcurrentBrowsers`), not a global cap.

#### Scenario: Successful container start — pool hit (org verified)
- **WHEN** `POST /browser/start` is called with a valid `account_id` that belongs to the caller's org and a warm slot exists for that account
- **THEN** the system claims the slot, returns HTTP 200 with `{ "status": "running", "cdp_port", "vnc_port", "container_id", "account_id" }`, and triggers background replenishment

#### Scenario: Successful container start submission — pool miss (org verified)
- **WHEN** `POST /browser/start` is called with a valid `account_id` that belongs to the caller's org and no warm slot exists and the queue is not full
- **THEN** the system enqueues a scheduler job and returns HTTP 202 with `{ "job_id", "status": "pending", "position": N, "account_id" }`

#### Scenario: Start rejected for foreign account
- **WHEN** `POST /browser/start` is called with an `account_id` that belongs to a different org than the caller's
- **THEN** the system returns HTTP 403 with `{ "error": "account does not belong to your organization" }` without touching the pool or queue

#### Scenario: Start when org concurrency cap reached but queue has space
- **WHEN** `POST /browser/start` is called and the caller org's running container count equals `org.EffectiveQuota().MaxConcurrentBrowsers` but the queue is not full
- **THEN** the system returns HTTP 202 with `{ "job_id", "status": "pending", "position": N }` — the job waits in the org's semaphore queue

#### Scenario: Start when container already running (idempotent)
- **WHEN** `POST /browser/start` is called for an account that already has a job in state `running`
- **THEN** the system returns HTTP 200 with `{ "job_id", "status": "running", "cdp_port", "vnc_port", "container_id" }` and does not claim a pool slot or enqueue a new job

#### Scenario: Profile directory created if absent
- **WHEN** the warm pool starts a container for an account whose profile directory does not exist
- **THEN** the system creates `data/profiles/account_{id}/` before starting the container
