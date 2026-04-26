## MODIFIED Requirements

### Requirement: Start browser container for an account
The system SHALL route `POST /browser/start` through `BrowserRuntimeManager` instead of directly calling `DockerBrowserService`. The manager inserts or updates a `browser_containers` row and transitions through the FSM (`pending → creating → starting → running`). On org mismatch the system SHALL return HTTP 403. On a warm pool hit the system SHALL return container info synchronously (HTTP 200) and update the `browser_containers` row to `running`. On a pool miss the system SHALL submit to the durable job scheduler and return HTTP 202. The concurrency cap enforced is the caller org's effective quota (`org.EffectiveQuota().MaxConcurrentBrowsers`).

#### Scenario: Successful container start — pool hit (org verified, FSM tracked)
- **WHEN** `POST /browser/start` is called with a valid `account_id` belonging to the caller's org and a warm slot exists for that account
- **THEN** the system claims the slot, inserts a `browser_containers` row with `state='running'`, starts the health probe, and returns HTTP 200 with `{ "status": "running", "cdp_port", "vnc_port", "container_id", "account_id" }`

#### Scenario: Successful container start submission — pool miss (org verified, FSM tracked)
- **WHEN** `POST /browser/start` is called with a valid `account_id` belonging to the caller's org and no warm slot exists
- **THEN** the system inserts a `browser_containers` row with `state='pending'`, submits a `browser_start` job to the scheduler, and returns HTTP 202 with `{ "job_id", "status": "pending", "account_id" }`

#### Scenario: Start rejected for foreign account
- **WHEN** `POST /browser/start` is called with an `account_id` belonging to a different org
- **THEN** the system returns HTTP 403 with `{ "error": "account does not belong to your organization" }` without modifying `browser_containers`

#### Scenario: Start when container already running (idempotent)
- **WHEN** `POST /browser/start` is called for an account whose `browser_containers` row has `state='running'`
- **THEN** the system returns HTTP 200 with `{ "status": "running", "cdp_port", "vnc_port", "container_id" }` without starting a new container or modifying the FSM state

#### Scenario: Profile directory created if absent
- **WHEN** the manager starts a container for an account whose profile directory does not exist
- **THEN** the system creates `data/profiles/account_{id}/` before any Docker create call

### Requirement: Stop browser container for an account
The system SHALL route `POST /browser/stop` through `BrowserRuntimeManager`. The manager transitions the container FSM through `running → stopping → stopped → removed`, calls `DockerBrowserService.Stop`, marks the associated `scheduler_jobs` row as `completed`, and releases the org semaphore slot. Intentional stops SHALL NOT trigger the restart policy.

#### Scenario: Successful container stop
- **WHEN** `POST /browser/stop` is called for an account whose `browser_containers` row has `state='running'`
- **THEN** the manager transitions the row to `stopping`, stops the health probe goroutine, calls `DockerBrowserService.Stop`, transitions to `stopped` then `removed`, marks the `scheduler_jobs` row `completed`, releases the org semaphore slot, and returns HTTP 200

#### Scenario: Stop for account with no running container
- **WHEN** `POST /browser/stop` is called for an account whose `browser_containers` row is in a terminal state (`stopped`, `removed`) or does not exist
- **THEN** the system returns HTTP 404 with `{ "error": "no running container for account" }`

#### Scenario: Stop rejected for foreign account
- **WHEN** `POST /browser/stop` is called with an `account_id` belonging to a different org
- **THEN** the system returns HTTP 403 without modifying any state

#### Scenario: Restart policy not evaluated on intentional stop
- **WHEN** `POST /browser/stop` completes and the container reaches `stopped` state via this path
- **THEN** `browser_containers.restart_count` is NOT incremented; no `browser_start` job is submitted
