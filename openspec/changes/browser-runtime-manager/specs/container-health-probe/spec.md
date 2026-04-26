## ADDED Requirements

### Requirement: Per-container CDP health check
The system SHALL run a dedicated probe goroutine for each container in `running` state. The probe SHALL issue an HTTP GET to `http://localhost:<cdpPort>/json/version` with a timeout of `CONTAINER_HEALTH_TIMEOUT` (default 3s) every `CONTAINER_HEALTH_PROBE_INTERVAL` (default 10s). On success it SHALL update `browser_containers.last_health_at` and `last_health_ok=1`. On failure it SHALL update `last_health_ok=0`.

#### Scenario: Healthy container probe
- **WHEN** the CDP `/json/version` endpoint returns HTTP 200 within `CONTAINER_HEALTH_TIMEOUT`
- **THEN** `last_health_at` is updated to now and `last_health_ok` is set to 1; the container remains in `running` state

#### Scenario: Unhealthy container after consecutive failures
- **WHEN** the CDP health check fails `CONTAINER_HEALTH_FAIL_THRESHOLD` (default 2) consecutive times
- **THEN** the probe goroutine calls `manager.Transition(accountID, running → unhealthy)` and exits; the restart policy is evaluated

#### Scenario: Probe starts after startup grace period
- **WHEN** a container transitions to `running` state
- **THEN** the probe goroutine waits `CONTAINER_HEALTH_PROBE_INTERVAL * 2` before issuing the first health check, allowing Chrome to finish initialization

### Requirement: Probe lifecycle tied to container state
The system SHALL start a probe goroutine exactly when a container enters `running` state and stop it when the container leaves `running` or `unhealthy` state (enters `stopping`, `stopped`, or `removed`).

#### Scenario: Probe goroutine started on running entry
- **WHEN** `BrowserRuntimeManager` transitions a container to `running`
- **THEN** `ContainerHealthProbe.Start(accountID, cdpPort)` is called; a new probe goroutine is registered in the probe map under `accountID`

#### Scenario: Probe goroutine stopped on stopping entry
- **WHEN** `BrowserRuntimeManager` transitions a container to `stopping`
- **THEN** `ContainerHealthProbe.Stop(accountID)` cancels the goroutine's context; the goroutine exits without triggering further transitions

#### Scenario: No duplicate probe goroutines
- **WHEN** `ContainerHealthProbe.Start(accountID, cdpPort)` is called for an `accountID` that already has a running probe
- **THEN** the existing probe is cancelled first; a new probe goroutine is started with the updated port

### Requirement: Health check configuration
The system SHALL read `CONTAINER_HEALTH_PROBE_INTERVAL` (default `10s`) and `CONTAINER_HEALTH_TIMEOUT` (default `3s`) from the environment. `CONTAINER_HEALTH_TIMEOUT` SHALL be strictly less than `CONTAINER_HEALTH_PROBE_INTERVAL`.

#### Scenario: Invalid timeout configuration
- **WHEN** `CONTAINER_HEALTH_TIMEOUT >= CONTAINER_HEALTH_PROBE_INTERVAL`
- **THEN** the system logs a warning and clamps `CONTAINER_HEALTH_TIMEOUT` to `CONTAINER_HEALTH_PROBE_INTERVAL / 2`
