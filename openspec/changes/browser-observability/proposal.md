## Why

The browser platform now spans four interconnected subsystems (Docker container lifecycle, scheduler queue, warm pool, and port registry), but none of them emit structured logs or metrics — operators have no way to distinguish a slow start from a crash loop, know when the queue is backing up, or understand resource consumption per container. Adding observability now, before the system reaches production scale, prevents blind spots from becoming incidents.

## What Changes

- Introduce a `MetricsCollector` component that maintains real-time gauge and counter values across all subsystems, exposed via `GET /metrics` in both a plain JSON format and an optional Prometheus text format.
- Add a `ContainerStatsPoller` goroutine that queries the Docker stats API for each running container on a configurable interval, recording CPU percentage and memory usage per account.
- Emit structured log lines (JSON, with `account_id`, `container_id`, `event`, `duration_ms`) on every lifecycle event: container start, stop, crash detection, orphan recovery, pool claim, pool eviction, lease expiry, queue enqueue, job state transition.
- Add an `AlertManager` component that evaluates configurable thresholds against live metrics and emits WARN log entries (and optionally Telegram messages) for: pool exhaustion, queue backlog spike, and container crash-loop detection.
- Add `GET /metrics` endpoint (JSON default, Prometheus text via `Accept: text/plain;version=0.0.4` or `?format=prometheus`).
- Add `GET /browser/health` liveness endpoint returning overall system health status.

## Capabilities

### New Capabilities

- `browser-metrics`: Real-time metric gauges and counters (active containers, queued jobs, pool depth, failure rate, per-container CPU/memory) collected in-process and exposed via REST.
- `browser-lifecycle-logging`: Structured JSON log emission on every lifecycle event across all browser subsystems, with consistent fields (`account_id`, `container_id`, `event`, `duration_ms`, `error`).
- `browser-alerting`: Threshold-based alert evaluation against live metrics; alerts emitted as WARN logs and optionally forwarded to the existing Telegram bot.
- `container-stats-collection`: Per-container CPU and memory polling via Docker stats API, aggregated into the metrics store.

### Modified Capabilities

<!-- No existing specs are changing at the requirements level — observability is additive. -->

## Impact

- **Code**: New `internal/observability/metrics.go`, `internal/observability/lifecycle_log.go`, `internal/observability/alert_manager.go`, `internal/observability/stats_poller.go`; `internal/server/browser_handlers.go` and `api.go` get new routes; all existing subsystems (`DockerBrowserService`, `Scheduler`, `WarmPool`, `PortRegistry`) receive a logger/metrics reference.
- **APIs**: New `GET /metrics` and `GET /browser/health` endpoints. No existing endpoints change.
- **Dependencies**: Prometheus client optional — `github.com/prometheus/client_golang` added behind a build tag `prometheus`. JSON metrics require no new deps. Docker stats API uses the already-present `github.com/docker/docker/client`.
- **Config**: New env vars `METRICS_FORMAT` (default `json`, or `prometheus`), `STATS_POLL_INTERVAL` (default `15s`), `ALERT_POOL_EXHAUSTION_THRESHOLD` (default `0.9` = 90% capacity), `ALERT_QUEUE_BACKLOG_THRESHOLD` (default `100`), `ALERT_CRASH_LOOP_WINDOW` (default `5m`), `ALERT_CRASH_LOOP_COUNT` (default `3`), `ALERT_TELEGRAM_ENABLED` (default `false`).
- **Logging**: Existing `log.Printf` calls are NOT mass-replaced — only lifecycle paths in the browser subsystem get structured logs. Application-level logs remain as-is.
