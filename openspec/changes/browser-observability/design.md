## Context

The browser platform after all prior changes looks like this:

```
HTTP handler
  → WarmPool.Claim()          (no logs, no metrics)
  → Scheduler.Submit()        (no logs, no metrics)
    → worker → DockerBrowserService.Start()  (log.Printf only)
PortRegistry.Acquire/Release  (no logs, no metrics)
ContainerStatsPoller          (doesn't exist yet)
```

Each subsystem runs in isolation. When a container fails to start, the only signal is a Go error returned to the handler. No structured event trail exists. The existing logs are unstructured `log.Printf` calls with inconsistent field names. The Telegram bot receives no runtime alerts.

The system uses Gofiber v2 for HTTP, the Docker SDK for container management, and the existing Telegram bot (`gopkg.in/telebot.v3`) for staff communication. The Prometheus client library is available but not yet imported.

## Goals / Non-Goals

**Goals:**
- Single `MetricsCollector` struct accessible to all subsystems via dependency injection; no global variables.
- All browser lifecycle events produce a structured JSON log line to stdout with a fixed schema.
- `GET /metrics` returns real-time snapshot in JSON (default) or Prometheus text format.
- `GET /browser/health` returns overall health (OK / DEGRADED / UNHEALTHY) based on live metric thresholds.
- `ContainerStatsPoller` queries Docker stats API every `STATS_POLL_INTERVAL` for all running containers.
- `AlertManager` evaluates thresholds after each metrics update; emits WARN log; optionally sends Telegram message (rate-limited to one alert per alert-type per 5 minutes).
- Crash-loop detection: if the same `account_id` has ≥ `ALERT_CRASH_LOOP_COUNT` failed starts within `ALERT_CRASH_LOOP_WINDOW`, an alert fires.

**Non-Goals:**
- Replacing the existing `log.Printf` calls outside the browser subsystem.
- Distributed tracing (OpenTelemetry spans).
- Persisting metrics to a time-series database (InfluxDB, VictoriaMetrics) — that's a future ops concern.
- A metrics dashboard UI in the web app — external Prometheus + Grafana covers this.
- Alert routing to anything other than logs and Telegram.

## Decisions

### 1. In-process metric store using atomic integers and a RWMutex snapshot map

**Decision**: `MetricsCollector` holds:
- `atomic.Int64` for gauges that change frequently: `activeContainers`, `queuedJobs`, `warmPoolDepth`, `allocatedPorts`.
- `atomic.Int64` for counters that only increment: `containerStartsTotal`, `containerStopsTotal`, `containerFailuresTotal`, `portsExpiredTotal`.
- `sync.RWMutex`-guarded `containerStats map[int64]ContainerStat` for per-container CPU/memory snapshots (written by poller, read by metrics endpoint).

**Why**: Atomic integers are zero-allocation reads and writes — the metrics endpoint can snapshot them without a lock. Per-container stats need a map keyed by `account_id` and are written infrequently (every `STATS_POLL_INTERVAL`) so a RWMutex is appropriate.

**Alternative considered**: `github.com/prometheus/client_golang` registry as the sole store — rejected because it couples the metrics store to Prometheus even when `METRICS_FORMAT=json`. The decoupled store serializes to either format.

### 2. Prometheus client used only for text-format serialization, not as the metric registry

**Decision**: When `?format=prometheus` is requested, `GET /metrics` iterates the `MetricsCollector` state and writes Prometheus text format manually using `fmt.Fprintf`. The `prometheus/client_golang` package is **not** imported; format generation uses the well-documented line format (`# HELP`, `# TYPE`, metric lines).

**Why**: The Prometheus text format is a simple line protocol. Hand-writing it avoids a `~2MB` import and its global default registry side-effects. Keeps the binary lean for deployments that only use JSON metrics.

**Alternative considered**: Import `prometheus/client_golang` behind a build tag — possible but adds complexity for a format that is a straightforward line protocol. Revisit if the metric set grows large enough that hand-serialization becomes error-prone.

### 3. Structured lifecycle logger as a thin wrapper over `log/slog` (Go 1.21+)

**Decision**: `LifecycleLogger` wraps `slog.Logger` configured with `slog.NewJSONHandler(os.Stdout, nil)`. Each log call takes an `Event` struct:
```go
type Event struct {
    Event       string  // "container_start", "container_stop", "crash_detected", etc.
    AccountID   int64
    ContainerID string
    DurationMs  int64
    Error       string  // empty if no error
    Extra       map[string]any // optional extra fields
}
```
`LifecycleLogger.Log(ctx, event Event)` calls `slog.InfoContext` or `slog.WarnContext` based on whether `Error` is set.

**Why**: `log/slog` is in the standard library since Go 1.21 (project uses Go 1.26). No additional import. JSON handler output is directly ingestible by Loki, Datadog, or any log aggregator.

**Alternative considered**: `github.com/uber-go/zap` — faster but adds a dependency and the log volume here is low (one line per lifecycle event, not per request).

### 4. `ContainerStatsPoller` calls Docker stats API non-streaming (single fetch per container)

**Decision**: The Docker stats API supports a streaming mode and a one-shot mode (`?stream=false`). The poller uses one-shot mode: for each running container, it calls `client.ContainerStats(ctx, id, false)`, reads one JSON frame, decodes CPU/memory, and updates `MetricsCollector.containerStats`. It runs on a ticker every `STATS_POLL_INTERVAL`.

**Why**: Streaming stats per container would require one persistent HTTP connection per container — 100 containers = 100 open connections. One-shot per tick is a clean request/response cycle, trivially parallelizable (one goroutine per container per tick with a `sync.WaitGroup`), and sufficient for a 15s poll interval.

**Alternative considered**: cAdvisor sidecar — offloads stats collection but adds infra complexity. Docker stats API is already available via the SDK and requires no new infra.

### 5. `AlertManager` uses a rate-limit map to prevent alert storms

**Decision**: `AlertManager` holds `lastAlertAt map[AlertType]time.Time` under a mutex. Before firing an alert, it checks `time.Since(lastAlertAt[type]) < 5 * time.Minute`. If suppressed, the condition is still logged at DEBUG. If fired, it logs at WARN and (if enabled) sends a Telegram message via the existing bot reference.

**Why**: Without rate limiting, a sustained pool exhaustion event would flood the Telegram chat and logs every metrics evaluation cycle. 5 minutes is long enough to avoid spam and short enough to remain actionable.

### 6. Crash-loop detection via a sliding window counter per account

**Decision**: `AlertManager` maintains `failureWindows map[int64]*ring.Ring` (Go `container/ring`), one ring buffer per `account_id` storing the timestamps of recent failures. On each failure event, the timestamp is pushed. The alert fires if the count of entries within `ALERT_CRASH_LOOP_WINDOW` reaches `ALERT_CRASH_LOOP_COUNT`.

**Why**: A ring buffer of fixed size (`ALERT_CRASH_LOOP_COUNT`) is memory-bounded — it holds at most N timestamps per account. Checking whether all N timestamps fall within the window is O(N). No external state store needed.

**Alternative considered**: A simple counter reset on a timer — rejected because it misses bursts that straddle timer boundaries (e.g., 2 failures just before reset + 2 just after = 4 failures, none detected).

### 7. Health endpoint uses threshold-based status, not ping

**Decision**: `GET /browser/health` does NOT check Docker connectivity or DB connectivity (those are separately monitored). It evaluates:
- `UNHEALTHY`: `activeContainers == 0 AND queuedJobs > 0` (scheduler stuck) OR `containerFailuresTotal` rate in last minute > 50%.
- `DEGRADED`: pool depth = 0 OR queue depth > `ALERT_QUEUE_BACKLOG_THRESHOLD`.
- `OK`: otherwise.

**Why**: Liveness/readiness probes in Kubernetes check if the process should be restarted — a Docker connectivity failure doesn't mean the process should restart (it would reconnect). The health status reflects observable user impact.

## Risks / Trade-offs

- **Docker stats API adds latency to the poller tick** → Mitigation: poller runs in a separate goroutine and never blocks request handling; stats are best-effort (stale data is acceptable).
- **`containerStats` map grows unbounded if containers are never removed** → Mitigation: `DockerBrowserService.Stop()` calls `MetricsCollector.RemoveContainerStats(accountID)` to delete the entry.
- **Structured log volume at scale** → At 100 containers starting/stopping per minute, ~200 log lines/min — negligible. No sampling needed at this scale.
- **Telegram bot rate limits (30 msg/s global, 1 msg/s per chat)** → Mitigation: `AlertManager` rate-limits to one alert per type per 5 minutes; well within Telegram limits.
- **`log/slog` requires Go 1.21+** → Project is on Go 1.26; not a concern.

## Migration Plan

1. Create `internal/observability/` package with `metrics.go`, `lifecycle_log.go`, `alert_manager.go`, `stats_poller.go`.
2. Update `DockerBrowserService`, `Scheduler`, `WarmPool`, `PortRegistry` constructors to accept `*MetricsCollector` and `*LifecycleLogger` (pass nil-safe no-op implementations initially for backward compatibility).
3. Instrument each subsystem with metric updates and log calls.
4. Add `GET /metrics` and `GET /browser/health` routes.
5. Wire everything in `cmd/scraper/main.go`.
6. Deploy: fully additive, no data migration, no API breaking changes.
7. Rollback: remove the `observability` package import — all other code unchanged.

## Open Questions

- Should `GET /metrics` require JWT authentication? Currently all `/browser/*` routes are JWT-protected. Prometheus scrapers typically use a separate bearer token. Proposed: protect with JWT for now; add scraper-specific token in a follow-up.
- Should the `ContainerStatsPoller` also record network I/O (bytes sent/received)? Useful for bandwidth monitoring. Deferred — not in the initial requirements.
- What is the appropriate `STATS_POLL_INTERVAL` default? 15s is proposed — fine-grained enough for alerting, coarse enough to not hammer Docker.
