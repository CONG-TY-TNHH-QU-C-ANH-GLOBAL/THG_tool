## ADDED Requirements

### Requirement: Structured JSON lifecycle log format
The system SHALL emit one structured JSON log line to stdout for each browser lifecycle event. Every log line SHALL include the fields: `time` (RFC3339), `level` (`INFO` or `WARN`), `event` (event name string), `account_id` (int64), `container_id` (string, empty if not yet assigned), `duration_ms` (int64, 0 if not applicable), and `error` (string, empty if no error).

#### Scenario: Successful container start logged
- **WHEN** `DockerBrowserService.Start` completes successfully for account 42
- **THEN** a JSON log line is emitted with `"event": "container_start"`, `"account_id": 42`, `"container_id": "<id>"`, `"duration_ms": <elapsed>`, `"error": ""`, `"level": "INFO"`

#### Scenario: Failed container start logged
- **WHEN** `DockerBrowserService.Start` returns an error for account 42
- **THEN** a JSON log line is emitted with `"event": "container_start_failed"`, `"account_id": 42`, `"container_id": ""`, `"error": "<error message>"`, `"level": "WARN"`

#### Scenario: Container stop logged
- **WHEN** `DockerBrowserService.Stop` is called for account 42
- **THEN** a JSON log line is emitted with `"event": "container_stop"`, `"account_id": 42`, `"container_id": "<id>"`, `"duration_ms": <elapsed>`, `"level": "INFO"`

### Requirement: Lifecycle events covered
The system SHALL emit structured log lines for ALL of the following events: `container_start`, `container_start_failed`, `container_stop`, `orphan_detected`, `orphan_removed`, `pool_slot_started`, `pool_slot_claimed`, `pool_slot_evicted`, `pool_slot_replenished`, `job_enqueued`, `job_scheduled`, `job_running`, `job_failed`, `job_completed`, `lease_expired`, `port_acquired`, `port_released`.

#### Scenario: Orphan detection logged
- **WHEN** a container with label `thg.account_id` is found at startup without a registry entry
- **THEN** a log line is emitted with `"event": "orphan_detected"` followed by `"event": "orphan_removed"` after the container is stopped

#### Scenario: Warm pool claim logged
- **WHEN** a warm slot for account 42 is claimed by `POST /browser/start`
- **THEN** a log line is emitted with `"event": "pool_slot_claimed"`, `"account_id": 42`, `"duration_ms"` reflecting pool lookup time

#### Scenario: Lease expiry logged
- **WHEN** the port registry reaper reclaims an expired lease
- **THEN** a log line is emitted with `"event": "lease_expired"`, `"account_id": <id>`, and the port pair in the `Extra` field

### Requirement: Log level respects error presence
The system SHALL emit `INFO`-level log lines for successful events and `WARN`-level log lines for events involving errors, failures, orphan detection, or alert conditions.

#### Scenario: Success event at INFO level
- **WHEN** a lifecycle event completes without error
- **THEN** the log line `"level"` field is `"INFO"`

#### Scenario: Error event at WARN level
- **WHEN** a lifecycle event includes a non-empty `error` field
- **THEN** the log line `"level"` field is `"WARN"`
