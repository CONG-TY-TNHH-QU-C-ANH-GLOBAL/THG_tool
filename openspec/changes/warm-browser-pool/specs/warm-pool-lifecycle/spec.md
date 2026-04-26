## ADDED Requirements

### Requirement: Pool fills to target size on startup
The system SHALL pre-warm Docker Chrome containers for configured accounts on service startup, up to `min(WARM_POOL_SIZE, MAX_CONCURRENT_BROWSERS)`. Each warm container SHALL have the target account's profile directory mounted and SHALL hold a `PortRegistry` allocation and a scheduler semaphore slot.

#### Scenario: Startup with capacity available
- **WHEN** the service starts and `WARM_POOL_SIZE=3` and 3+ active accounts exist and semaphore capacity allows
- **THEN** 3 containers are started (one per account), each counted against `MAX_CONCURRENT_BROWSERS`, and the pool reports depth 3

#### Scenario: Startup capped by MAX_CONCURRENT_BROWSERS
- **WHEN** `WARM_POOL_SIZE=5` but `MAX_CONCURRENT_BROWSERS=2`
- **THEN** at startup a warning is logged and the pool fills only 2 slots; `WARM_POOL_SIZE` is effectively capped to 2

#### Scenario: Startup with fewer accounts than pool size
- **WHEN** `WARM_POOL_SIZE=5` but only 2 active accounts exist
- **THEN** the pool fills 2 slots (one per account) and reports depth 2

### Requirement: Instant container assignment on pool hit
The system SHALL assign a pre-warmed container to an account request in under 100ms when a warm slot exists for that account, returning the container's existing port and ID information without starting a new container.

#### Scenario: Pool hit for the requesting account
- **WHEN** `POST /browser/start` is called for `account_id=42` and a warm slot exists for account 42
- **THEN** the system removes the slot from the pool, returns HTTP 200 with `{ "status": "running", "cdp_port", "vnc_port", "container_id", "account_id" }` within 100ms, and does not enqueue a scheduler job

#### Scenario: Pool miss falls through to scheduler
- **WHEN** `POST /browser/start` is called for `account_id=42` and no warm slot exists for account 42
- **THEN** the system falls through to `Scheduler.Submit()` and returns HTTP 202 with a job reference

#### Scenario: Already-running container bypasses both pool and scheduler
- **WHEN** `POST /browser/start` is called for an account that already has a running container (not a warm slot)
- **THEN** the system returns HTTP 200 with the existing container info and neither claims a pool slot nor enqueues a job

### Requirement: Auto-replenishment after slot assignment
The system SHALL asynchronously start a new warm container for an account whose slot was just claimed, so the next request for the same account is also fast. Replenishment SHALL be skipped if the pool is already at capacity or the semaphore has no available slots.

#### Scenario: Replenishment after pool hit
- **WHEN** a warm slot for account 42 is claimed
- **THEN** within 5 seconds a new container is started for account 42 and added to the pool (if capacity allows), without blocking the API response

#### Scenario: Replenishment skipped at capacity
- **WHEN** a warm slot is claimed and `MAX_CONCURRENT_BROWSERS` containers are already running (including other warm slots)
- **THEN** no replenishment container is started; the pool operates below target size until a slot is freed

#### Scenario: Replenishment queue does not overflow
- **WHEN** multiple slots are claimed simultaneously (burst)
- **THEN** each replenishment is enqueued in a buffered channel; no replenishment events are dropped and no goroutines are leaked

### Requirement: Pool account selection
The system SHALL determine which accounts to pre-warm using the `WARM_POOL_ACCOUNTS` env var (comma-separated account IDs) if set, otherwise selecting up to `WARM_POOL_SIZE` accounts from the DB ordered by most recently active.

#### Scenario: Explicit account list configured
- **WHEN** `WARM_POOL_ACCOUNTS=1,2,3` is set and `WARM_POOL_SIZE=3`
- **THEN** the pool pre-warms exactly accounts 1, 2, and 3

#### Scenario: Auto-selection by recent activity
- **WHEN** `WARM_POOL_ACCOUNTS` is not set and `WARM_POOL_SIZE=2`
- **THEN** the pool pre-warms the 2 most-recently-active accounts from the DB

#### Scenario: Configured account no longer in DB
- **WHEN** `WARM_POOL_ACCOUNTS` lists an account ID that does not exist in the DB
- **THEN** that account ID is skipped with a log warning; remaining valid accounts are warmed normally
