## 1. Config and Environment

- [ ] 1.1 Add to `internal/config/config.go`: `MetricsFormat` (default `json`), `StatsPollInterval` (default `15s`), `AlertPoolExhaustionThreshold` (default `0.9`), `AlertQueueBacklogThreshold` (default `100`), `AlertCrashLoopWindow` (default `5m`), `AlertCrashLoopCount` (default `3`), `AlertTelegramEnabled` (default `false`)
- [ ] 1.2 Add all new env vars to `.env.example` with documented defaults

## 2. Metrics Collector

- [ ] 2.1 Create `internal/observability/metrics.go` — `MetricsCollector` struct with `atomic.Int64` fields: `activeContainers`, `queuedJobs`, `warmPoolDepth`, `allocatedPorts`, `containerStartsTotal`, `containerStopsTotal`, `containerFailuresTotal`, `portsExpiredTotal`, `poolClaimsTotal`, `poolMissesTotal`
- [ ] 2.2 Add `containerStats sync.Map` (key `int64` accountID → `ContainerStat{ContainerID, CPUPercent, MemoryBytes, UptimeSeconds}`) with `SetContainerStats(accountID, stat)` and `RemoveContainerStats(accountID)` and `ListContainerStats() []ContainerStatEntry`
- [ ] 2.3 Implement `MetricsCollector.Snapshot() MetricsSnapshot` — reads all atomic fields and snapshots `containerStats` map under a single logical moment
- [ ] 2.4 Implement `MetricsCollector.SerializeJSON(snapshot) []byte` — marshal `MetricsSnapshot` to JSON
- [ ] 2.5 Implement `MetricsCollector.SerializePrometheus(snapshot) []byte` — hand-write Prometheus text format with `# HELP`, `# TYPE`, metric lines, and per-container gauge lines with `account_id` label
- [ ] 2.6 Write unit tests: atomic increments are race-free (`-race`), Snapshot is consistent, JSON output contains all fields, Prometheus output is parseable

## 3. Lifecycle Logger

- [ ] 3.1 Create `internal/observability/lifecycle_log.go` — `Event` struct and `LifecycleLogger` wrapping `slog.Logger` with `slog.NewJSONHandler(os.Stdout, nil)`
- [ ] 3.2 Implement `LifecycleLogger.Log(ctx context.Context, e Event)` — call `slog.InfoContext` if `e.Error == ""`, else `slog.WarnContext`; include all `Event` fields as slog attributes
- [ ] 3.3 Define all event name constants as typed `const` strings: `EventContainerStart`, `EventContainerStartFailed`, `EventContainerStop`, `EventOrphanDetected`, `EventOrphanRemoved`, `EventPoolSlotStarted`, `EventPoolSlotClaimed`, `EventPoolSlotEvicted`, `EventPoolSlotReplenished`, `EventJobEnqueued`, `EventJobScheduled`, `EventJobRunning`, `EventJobFailed`, `EventJobCompleted`, `EventLeaseExpired`, `EventPortAcquired`, `EventPortReleased`
- [ ] 3.4 Write unit tests: log output is valid JSON, level is `INFO` without error, level is `WARN` with error, all required fields are present

## 4. Container Stats Poller

- [ ] 4.1 Create `internal/observability/stats_poller.go` — `ContainerStatsPoller` struct with Docker client reference, `MetricsCollector` reference, `LifecycleLogger` reference, ticker interval
- [ ] 4.2 Implement `ContainerStatsPoller.Start(ctx context.Context)` — ticker goroutine; on each tick, get list of running containers from `DockerBrowserService.ListRunning()`, fan out to parallel goroutines (one per container) using `sync.WaitGroup`, each calls `dockerClient.ContainerStats(ctx, id, false)`, decodes CPU% and memory bytes, calls `collector.SetContainerStats`
- [ ] 4.3 Implement CPU% calculation from Docker stats: `(cpuDelta / systemDelta) * numCPU * 100` per Docker documentation
- [ ] 4.4 Ensure `ContainerStatsPoller` exits cleanly when `ctx` is cancelled
- [ ] 4.5 Write unit test with a mock Docker client: stats are stored in collector, error for one container does not abort others

## 5. Alert Manager

- [ ] 5.1 Create `internal/observability/alert_manager.go` — `AlertType` string enum and `AlertManager` struct with `MetricsCollector` ref, `LifecycleLogger` ref, Telegram bot ref, `lastAlertAt map[AlertType]time.Time`, `failureWindows map[int64]*ring.Ring` (using `container/ring`), mutex
- [ ] 5.2 Implement `AlertManager.Evaluate(ctx context.Context)` — called after each metrics update; evaluates all three alert conditions (pool exhaustion, queue backlog, crash loop)
- [ ] 5.3 Implement pool exhaustion check: `float64(allocatedPorts) / float64(totalPortCapacity) >= threshold`; if true and rate-limit allows, emit WARN log and (if enabled) Telegram message; emit recovery INFO log when condition clears
- [ ] 5.4 Implement queue backlog check: `queuedJobs > ALERT_QUEUE_BACKLOG_THRESHOLD`; emit alert per rate-limit
- [ ] 5.5 Implement crash-loop detection: `AlertManager.RecordFailure(accountID int64)` pushes `time.Now()` to the ring buffer for that account; `Evaluate` counts entries within `ALERT_CRASH_LOOP_WINDOW` and fires if count >= `ALERT_CRASH_LOOP_COUNT`
- [ ] 5.6 Implement `AlertManager.rateLimitAllow(alertType AlertType) bool` — returns true and updates `lastAlertAt` if 5 minutes have elapsed since last fire
- [ ] 5.7 Implement Telegram message send using existing bot: format as `"🚨 [AlertType] — <detail> (<UTC timestamp>)"`
- [ ] 5.8 Wire `AlertManager.RecordFailure(accountID)` call into `DockerBrowserService.Start` error path and scheduler job `failed` transition
- [ ] 5.9 Write unit tests: pool alert fires at threshold, suppressed within 5m, different types independent; crash-loop fires after N failures in window, not before; ring buffer is bounded

## 6. REST Handlers

- [ ] 6.1 Add `GET /metrics` handler in `internal/server/browser_handlers.go` — check `format` query param and `Accept` header; call `collector.Snapshot()` then `SerializeJSON` or `SerializePrometheus`; set correct `Content-Type`
- [ ] 6.2 Add `GET /browser/health` handler — evaluate `UNHEALTHY`/`DEGRADED`/`OK` from snapshot; return HTTP 503 for UNHEALTHY, HTTP 200 otherwise
- [ ] 6.3 Register both routes in `internal/server/api.go`

## 7. Subsystem Instrumentation

- [ ] 7.1 Update `DockerBrowserService.Start()` — log `EventContainerStart` / `EventContainerStartFailed`, increment `containerStartsTotal` or `containerFailuresTotal`, call `collector.SetContainerStats` after start, call `AlertManager.RecordFailure` on error
- [ ] 7.2 Update `DockerBrowserService.Stop()` — log `EventContainerStop`, increment `containerStopsTotal`, call `collector.RemoveContainerStats`
- [ ] 7.3 Update `DockerBrowserService.Reconcile()` — log `EventOrphanDetected` and `EventOrphanRemoved` per orphan
- [ ] 7.4 Update `Scheduler` worker — log `EventJobEnqueued`, `EventJobScheduled`, `EventJobRunning`, `EventJobFailed`, `EventJobCompleted`; update `queuedJobs` gauge on enqueue/dequeue
- [ ] 7.5 Update `WarmPool` — log `EventPoolSlotStarted`, `EventPoolSlotClaimed`, `EventPoolSlotEvicted`, `EventPoolSlotReplenished`; update `warmPoolDepth` gauge; increment `poolClaimsTotal` / `poolMissesTotal`
- [ ] 7.6 Update `PortRegistry` reaper — log `EventLeaseExpired`; increment `portsExpiredTotal`
- [ ] 7.7 Update `PortRegistry.Acquire` / `Release` — log `EventPortAcquired` / `EventPortReleased`; update `allocatedPorts` gauge

## 8. Wiring in main.go

- [ ] 8.1 Instantiate `MetricsCollector` and `LifecycleLogger` in `cmd/scraper/main.go`
- [ ] 8.2 Pass `MetricsCollector` and `LifecycleLogger` to `DockerBrowserService`, `Scheduler`, `WarmPool`, `PortRegistry` constructors
- [ ] 8.3 Instantiate `AlertManager` with `MetricsCollector`, `LifecycleLogger`, and Telegram bot ref
- [ ] 8.4 Instantiate `ContainerStatsPoller` and call `poller.Start(ctx)` after services are running
- [ ] 8.5 Start a goroutine that calls `AlertManager.Evaluate(ctx)` every `STATS_POLL_INTERVAL` (piggybacking on the stats poll cadence)
- [ ] 8.6 Ensure `ContainerStatsPoller` and `AlertManager` goroutines stop cleanly on shutdown via context cancellation

## 9. Verification

- [ ] 9.1 `go build ./cmd/scraper/` passes with no new warnings
- [ ] 9.2 `go test ./internal/observability/...` — all tests pass with `-race`
- [ ] 9.3 Manual test: start service, start 2 containers, call `GET /metrics` — confirm `active_containers=2` and `container_stats` has 2 entries with non-zero CPU and memory
- [ ] 9.4 Manual test: call `GET /metrics?format=prometheus` — paste output into `promtool check metrics` (if available) and confirm no format errors
- [ ] 9.5 Manual test: set `ALERT_QUEUE_BACKLOG_THRESHOLD=2`, enqueue 3 jobs while pool is at capacity — confirm WARN log line with `"event": "alert_queue_backlog"` appears within one evaluation cycle
- [ ] 9.6 Manual test: call `GET /browser/health` with no containers and pending jobs — confirm HTTP 503 with `"status": "unhealthy"`
