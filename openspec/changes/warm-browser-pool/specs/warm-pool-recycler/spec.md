## ADDED Requirements

### Requirement: Idle container eviction after timeout
The system SHALL stop and remove warm pool containers that have been idle (not assigned) for longer than `WARM_IDLE_TIMEOUT`, to prevent stale Chrome sessions, memory accumulation, and profile lock issues.

#### Scenario: Idle container evicted at timeout
- **WHEN** a warm slot has been idle for longer than `WARM_IDLE_TIMEOUT` (default 10 minutes)
- **THEN** the system stops and removes the container, releases its port pair and semaphore slot, and removes the slot from the pool

#### Scenario: Default idle timeout applied
- **WHEN** `WARM_IDLE_TIMEOUT` is not set in the environment
- **THEN** idle containers are evicted after 10 minutes of inactivity

#### Scenario: Container claimed before eviction
- **WHEN** a warm slot is claimed by `POST /browser/start` before the recycler tick runs
- **THEN** the container is NOT evicted (claim removes it from the pool atomically before the recycler can see it)

### Requirement: Automatic slot replacement after eviction
The system SHALL add the evicted account to the replenishment queue after recycling, so a fresh warm container is started for that account (subject to capacity).

#### Scenario: Slot replaced after eviction
- **WHEN** a warm slot is evicted for account 42
- **THEN** account 42 is enqueued for replenishment; a new container is started for account 42 within 5 seconds if capacity allows

#### Scenario: No replacement when at capacity
- **WHEN** a warm slot is evicted but `MAX_CONCURRENT_BROWSERS` containers are still running
- **THEN** no replacement container is started; the pool remains below target depth until capacity is freed

### Requirement: Account list refresh on each recycler tick
The system SHALL re-read the active account list from the DB on each recycler tick and warm any newly active accounts that are not yet in the pool (up to `WARM_POOL_SIZE`), and evict slots for accounts that are no longer active.

#### Scenario: New account becomes active
- **WHEN** an account is added to the DB and the recycler ticks
- **THEN** if pool depth < `WARM_POOL_SIZE` and capacity allows, a warm container is started for the new account

#### Scenario: Account deactivated while warm slot exists
- **WHEN** an account is marked inactive in the DB and a warm slot exists for it
- **THEN** on the next recycler tick the slot is evicted (container stopped, ports/semaphore released) and no replacement is started for that account

### Requirement: Pool status endpoint
The system SHALL expose `GET /browser/pool/status` returning the current state of all warm slots, including account assignment, idle duration, port allocations, and pool capacity.

#### Scenario: Pool with active slots
- **WHEN** `GET /browser/pool/status` is called with 2 warm slots active
- **THEN** the response is HTTP 200 with `{ "pool_size": WARM_POOL_SIZE, "slots": [{ "account_id", "container_id", "cdp_port", "vnc_port", "idle_seconds" }, ...], "depth": 2, "capacity": MAX_CONCURRENT_BROWSERS }`

#### Scenario: Empty pool
- **WHEN** `GET /browser/pool/status` is called and no warm slots exist
- **THEN** the response is `{ "pool_size": WARM_POOL_SIZE, "slots": [], "depth": 0, "capacity": MAX_CONCURRENT_BROWSERS }`
