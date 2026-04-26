## ADDED Requirements

### Requirement: Per-container CPU and memory polling
The system SHALL poll the Docker stats API for every running container on each tick of `STATS_POLL_INTERVAL`, recording CPU usage percentage and memory usage in bytes per `account_id`. Stats SHALL be available in the metrics snapshot and in `GET /metrics`.

#### Scenario: Stats polled for running containers
- **WHEN** 3 containers are running and the poller ticks
- **THEN** CPU% and memory bytes are fetched for each container in parallel and stored in `MetricsCollector.containerStats`

#### Scenario: Stats not polled for stopped containers
- **WHEN** a container is stopped and the poller ticks
- **THEN** no Docker stats request is made for that container; its entry is removed from `MetricsCollector.containerStats`

#### Scenario: Default poll interval
- **WHEN** `STATS_POLL_INTERVAL` is not set
- **THEN** the poller ticks every 15 seconds

#### Scenario: Docker stats API error is non-fatal
- **WHEN** the Docker stats API returns an error for one container during a poll tick
- **THEN** that container's stats entry is left at its last known value, the error is logged at WARN, and polling continues for all other containers

### Requirement: Per-container stats in metrics response
The system SHALL include per-container stats in `GET /metrics` under a `container_stats` array, each entry containing `account_id`, `container_id`, `cpu_percent`, `memory_bytes`, and `uptime_seconds`.

#### Scenario: Container stats included in JSON metrics
- **WHEN** `GET /metrics` is called and 2 containers are running
- **THEN** the response JSON includes `"container_stats": [{ "account_id": N, "container_id": "...", "cpu_percent": X, "memory_bytes": Y, "uptime_seconds": Z }, ...]`

#### Scenario: Container stats as Prometheus gauge
- **WHEN** `GET /metrics?format=prometheus` is called
- **THEN** the response includes gauge metrics `browser_container_cpu_percent{account_id="N"}` and `browser_container_memory_bytes{account_id="N"}` for each running container

### Requirement: Stats cleared on container stop
The system SHALL remove a container's stats entry from `MetricsCollector.containerStats` when the container stops, so `GET /metrics` never returns stats for containers that are no longer running.

#### Scenario: Stats entry removed after stop
- **WHEN** `DockerBrowserService.Stop` is called for account 42
- **THEN** the entry for account 42 is deleted from `containerStats`; the next `GET /metrics` response does not include stats for account 42
