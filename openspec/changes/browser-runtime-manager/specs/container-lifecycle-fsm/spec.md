## ADDED Requirements

### Requirement: Formal container state machine
The system SHALL define the following valid container states: `pending`, `creating`, `starting`, `running`, `unhealthy`, `stopping`, `stopped`, `removed`. All state is persisted in the `browser_containers` SQLite table. The valid transitions SHALL be: `pending → creating`, `creating → starting`, `creating → removed` (create failed), `starting → running`, `starting → stopped` (start failed), `running → unhealthy`, `running → stopping`, `unhealthy → stopping`, `unhealthy → removed` (policy: never), `stopping → stopped`, `stopped → removed`, `stopped → creating` (restart).

#### Scenario: Only valid transitions accepted
- **WHEN** `BrowserRuntimeManager` attempts a state transition (e.g., `running → creating`)
- **THEN** the system executes `UPDATE browser_containers SET state=<new>, updated_at=? WHERE account_id=? AND state=<expected>`; if 0 rows are affected the transition is rejected with an error and the state is unchanged

#### Scenario: Concurrent transition race
- **WHEN** two goroutines simultaneously attempt to transition the same container from `running` to different states (`stopping` vs `unhealthy`)
- **THEN** exactly one UPDATE affects 1 row (SQLite serialized write); the other gets 0 rows and discards its attempt

### Requirement: `browser_containers` persistence table
The system SHALL maintain a `browser_containers` SQLite table with columns: `account_id INTEGER PRIMARY KEY`, `container_id TEXT`, `state TEXT`, `restart_count INTEGER DEFAULT 0`, `last_health_at DATETIME`, `last_health_ok INTEGER`, `cpu_quota INTEGER DEFAULT 0`, `memory_limit INTEGER DEFAULT 0`, `created_at DATETIME`, `updated_at DATETIME`.

#### Scenario: Row created on container start request
- **WHEN** `BrowserRuntimeManager.Start(accountID)` is called and no row exists for `account_id`
- **THEN** a new row is inserted with `state='pending'` and `restart_count=0` before any Docker API call

#### Scenario: Row updated on every transition
- **WHEN** any FSM transition completes
- **THEN** `state` and `updated_at` are updated atomically; `container_id` is set when the Docker container is created

### Requirement: Startup reconciliation
On `BrowserRuntimeManager.Start()`, the system SHALL reconcile `browser_containers` rows against actual Docker container state and apply corrections before accepting requests.

#### Scenario: DB row exists but Docker container missing
- **WHEN** a `browser_containers` row has `state` in `{creating, starting, running, unhealthy, stopping}` but the Docker container does not exist
- **THEN** the system transitions the row to `removed`; if the restart policy allows, a new `browser_start` job is submitted

#### Scenario: Docker container running but no DB row
- **WHEN** a Docker container matching the `thg-browser-<accountID>` naming pattern is found running but no `browser_containers` row exists for that `account_id`
- **THEN** the system inserts a new row with `state='running'` and `container_id` set; the health probe is started for that container

#### Scenario: DB row exists and Docker container exited
- **WHEN** a `browser_containers` row has `state='running'` but the Docker container reports `Status='exited'`
- **THEN** the system transitions the row to `stopped`; if the restart policy allows, a new `browser_start` job is submitted

### Requirement: Runtime status endpoint
The system SHALL expose `GET /browser/:id/runtime` returning `{ "account_id", "container_id", "state", "restart_count", "last_health_at", "last_health_ok", "cpu_quota", "memory_limit", "created_at", "updated_at" }` by reading the `browser_containers` row for the given account.

#### Scenario: Running container status
- **WHEN** `GET /browser/42/runtime` is called and a `browser_containers` row exists for `account_id=42`
- **THEN** the system returns HTTP 200 with the full row as JSON; no Docker API call is made

#### Scenario: No row for account
- **WHEN** `GET /browser/42/runtime` is called and no `browser_containers` row exists for `account_id=42`
- **THEN** the system returns HTTP 404 with `{"error":"no runtime record for account"}`
