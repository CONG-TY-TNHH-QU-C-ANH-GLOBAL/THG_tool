## ADDED Requirements

### Requirement: Real-time metrics endpoint
The system SHALL expose `GET /metrics` returning a snapshot of all current metric values. The default response format SHALL be JSON. When the request includes `?format=prometheus` or `Accept: text/plain;version=0.0.4`, the response SHALL be Prometheus text exposition format.

#### Scenario: JSON metrics returned by default
- **WHEN** `GET /metrics` is called without format parameters
- **THEN** the response is HTTP 200 with `Content-Type: application/json` and a JSON object containing all metric fields

#### Scenario: Prometheus format returned on request
- **WHEN** `GET /metrics?format=prometheus` is called
- **THEN** the response is HTTP 200 with `Content-Type: text/plain; version=0.0.4` and valid Prometheus text exposition format with `# HELP` and `# TYPE` lines for each metric

#### Scenario: Metrics snapshot is consistent
- **WHEN** `GET /metrics` is called
- **THEN** all gauge values in the response reflect the state at the moment of the request; counter values are monotonically non-decreasing across calls

### Requirement: Platform metric coverage
The system SHALL track and expose the following metrics at minimum: `active_containers` (gauge), `queued_jobs` (gauge), `warm_pool_depth` (gauge), `allocated_ports` (gauge), `container_starts_total` (counter), `container_stops_total` (counter), `container_failures_total` (counter), `ports_expired_total` (counter), `pool_claims_total` (counter), `pool_misses_total` (counter).

#### Scenario: Active container gauge reflects running count
- **WHEN** 5 containers are running and `GET /metrics` is called
- **THEN** `active_containers` equals 5

#### Scenario: Failure counter increments on start error
- **WHEN** `DockerBrowserService.Start` returns an error for a job
- **THEN** `container_failures_total` increments by 1 and is reflected in the next `GET /metrics` response

#### Scenario: Pool metrics reflect warm pool state
- **WHEN** the warm pool has 2 warm slots and `GET /metrics` is called
- **THEN** `warm_pool_depth` equals 2; after a slot is claimed, `warm_pool_depth` equals 1 and `pool_claims_total` has incremented by 1

### Requirement: System health endpoint
The system SHALL expose `GET /browser/health` returning an overall health status string (`ok`, `degraded`, or `unhealthy`) and a map of contributing factors.

#### Scenario: Healthy system
- **WHEN** containers are running, queue is empty, and pool has available slots
- **THEN** `GET /browser/health` returns HTTP 200 with `{ "status": "ok", "checks": { "queue": "ok", "pool": "ok", "scheduler": "ok" } }`

#### Scenario: Degraded — pool exhausted
- **WHEN** warm pool depth is 0 and `GET /browser/health` is called
- **THEN** the response includes `"status": "degraded"` and `"checks": { "pool": "exhausted" }`

#### Scenario: Degraded — queue backlog
- **WHEN** queued jobs exceed `ALERT_QUEUE_BACKLOG_THRESHOLD` and `GET /browser/health` is called
- **THEN** the response includes `"status": "degraded"` and `"checks": { "queue": "backlog" }`

#### Scenario: Unhealthy — scheduler stuck
- **WHEN** `active_containers == 0` and `queued_jobs > 0` for more than 30 seconds
- **THEN** `GET /browser/health` returns HTTP 503 with `"status": "unhealthy"` and `"checks": { "scheduler": "stuck" }`
