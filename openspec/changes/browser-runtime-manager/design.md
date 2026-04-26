## Context

The `docker-browser-service` change introduced `DockerBrowserService` to start and stop Chrome containers per Facebook account. That service is stateless: it issues Docker API calls and returns. If the container crashes, OOM-kills, or enters a partial state (created but not started), nothing detects or recovers it. The service also has no concept of resource quotas — a single runaway container can starve others.

The `browser-scheduler` and `idempotent-job-scheduler` changes queue and pace `browser_start` jobs but do not inspect container health after the job completes. Once a job is marked `running`, the scheduler has no further involvement; the container lives or dies unobserved.

Existing infrastructure: Docker SDK (`github.com/docker/docker/client`), SQLite (`modernc.org/sqlite`), `internal/store/store.go` for migrations. The `DockerBrowserService` is the single implementation of `BrowserServicer`; it will remain as the low-level Docker API facade while `BrowserRuntimeManager` wraps it.

## Goals / Non-Goals

**Goals:**
- Formal FSM: `pending → creating → starting → running → unhealthy → stopping → stopped → removed`. Every container has a persisted state in `browser_containers` SQLite table.
- Health probing: periodic CDP `/json/version` check per running container; transition to `unhealthy` on failure.
- Restart policy: `never`, `on-failure:<N>`, `always` — auto-requeue failed/unhealthy containers within the policy.
- Resource limits: CPU quota and memory enforced at Docker container creation via `HostConfig.NanoCPUs` and `HostConfig.Memory`.
- Startup reconciliation: on start, query Docker for actual container state and reconcile against `browser_containers` rows — detect orphans (running in Docker but no DB row) and ghosts (DB row with no Docker container).
- Expose `GET /browser/:id/runtime` with full lifecycle state, restart count, last health check, and resource snapshot.

**Non-Goals:**
- Distributed coordination across nodes — single-node Docker socket only.
- Container migration or live restart without stopping — stop-then-start is sufficient.
- Dynamic resource limit adjustment without container restart.
- Replacing the `DockerBrowserService` — the manager delegates all Docker API calls to it.
- Cgroup v1 vs v2 compatibility shims — rely on Docker's own abstraction.

## Decisions

### 1. FSM enforced in DB, not in memory

**Decision**: `browser_containers.state` is the authoritative FSM state. Every transition is an `UPDATE browser_containers SET state=?, updated_at=? WHERE account_id=? AND state=<expected>`. If the UPDATE affects 0 rows, the transition was illegal (concurrent mutation or wrong prior state) and an error is returned.

**Why**: An in-memory FSM (Go struct + mutex) is correct only within one process lifetime. After crash-restart the manager reads `browser_containers` to restore state. Storing state only in memory would require full reconciliation against Docker on every startup, which is racy for partially-transitioned containers.

**Alternative considered**: Optimistic locking with a `version` column — correct but more verbose; SQLite's serialized write lock provides the same protection with the simpler `WHERE state=<expected>` pattern.

### 2. `ContainerHealthProbe` runs one goroutine per running container

**Decision**: `ContainerHealthProbe` keeps a `map[accountID]cancelFunc` of in-flight probe goroutines (protected by `sync.Mutex`). On `running` state entry it spawns a goroutine that loops: HTTP GET `http://localhost:<cdpPort>/json/version` with a `CONTAINER_HEALTH_TIMEOUT`-bounded context, then sleep `CONTAINER_HEALTH_PROBE_INTERVAL`. On failure N consecutive times it calls `manager.Transition(accountID, running → unhealthy)`. On `stopping`/`stopped` state entry it calls the cancel function to kill the goroutine.

**Why**: One goroutine per container is simple and avoids the complexity of a single polling loop that must fan-out and collect results. At 20 concurrent containers (growth tier) this is 20 goroutines — negligible. At 100 (enterprise) it is still a non-issue.

**Alternative considered**: Single ticker that iterates all running containers — simpler goroutine count but harder to shut down individual probes and harder to test per-container behavior.

### 3. Restart policy: re-submit to `idempotent-job-scheduler`

**Decision**: When the restart policy allows a restart (`on-failure:<N>` and `restart_count < N`, or `always`), the manager calls `jobs.Submit("browser_start", "account:<id>", payload)` with a `run_after` delay. The idempotent job scheduler handles queuing, retries, and worker dispatch. The manager does not directly call `DockerBrowserService.Start`.

**Why**: Keeps the restart flow on the same durable path as initial starts. If the process crashes between detecting an unhealthy container and restarting it, the submitted job survives in `scheduler_jobs`. Avoids duplicating retry logic.

**Alternative considered**: Manager directly calls `DockerBrowserService.Start` on restart — simpler but not durable; a crash between `unhealthy` detection and container start loses the restart attempt.

### 4. Resource limits applied at creation, stored in `browser_containers`

**Decision**: `HostConfig.NanoCPUs = CONTAINER_CPU_QUOTA * 1e9` and `HostConfig.Memory = CONTAINER_MEMORY_LIMIT` are set in `DockerBrowserService.Start` when the values are non-zero. The effective limits are also stored in `browser_containers.cpu_quota` and `browser_containers.memory_limit` so `GET /browser/:id/runtime` can report them without a Docker inspect call.

**Why**: Docker inspect works but is an extra API call on every status request. Storing at creation time is one write; reads are free.

**Alternative considered**: Always call Docker inspect — accurate if limits change externally but adds ~5ms latency per status request; rejected.

### 5. Startup reconciliation: query Docker then patch DB

**Decision**: On `BrowserRuntimeManager.Start()`, call `DockerBrowserService.List()` to get all containers whose names match the `thg-browser-*` naming pattern. Cross-reference with all rows in `browser_containers`. For each mismatch:
- Row exists, Docker container missing → transition state to `removed`; trigger restart policy.
- Docker container exists, no row → insert orphan row with state `running`; trigger health probe.
- Row exists, Docker state `exited` → transition to `stopped`; trigger restart policy if applicable.

**Why**: Without reconciliation, a process crash leaves stale DB rows (containers removed out-of-band) or silent orphans (containers running with no DB tracking). Reconciliation makes startup idempotent regardless of prior crash state.

**Alternative considered**: Trust DB state on startup, only reconcile on explicit API call — simpler but leaves stale state indefinitely; rejected.

## Risks / Trade-offs

- **CDP health check connects to container host port** → Health probes reach containers via `localhost:<cdpPort>`. If the container is running but Chrome has not finished initializing, the probe fails, triggering unhealthy. Mitigation: apply a startup grace period of `CONTAINER_HEALTH_PROBE_INTERVAL * 2` after `running` state entry before starting probes.
- **`on-failure` restart re-submits to job scheduler which may queue behind other jobs** → A crashed container's restart is treated the same as a new start request. Under load the queue may delay the restart. Mitigation: set `run_after` to `now` (immediate) for restart submissions; the queue FIFO order ensures it is picked up as soon as a worker is free.
- **Docker inspect during reconciliation is O(n) serial API calls** → For 100 containers, reconciliation at startup scans all. Mitigation: use `DockerBrowserService.List()` which is a single Docker `GET /containers/json` call with a label filter; O(1) API calls regardless of container count.
- **FSM state mismatch under concurrent stop + health failure** → `stopping` and `unhealthy` transitions racing on the same row. Mitigation: `UPDATE … WHERE state=<expected>` returns 0 rows on stale state; the losing transition is discarded; the winner proceeds. All callers handle the 0-rows case as a no-op.

## Migration Plan

1. Add `browser_containers` table migration in `store.go`.
2. Implement `internal/browser/runtime_manager.go`, `health_probe.go`, `restart_policy.go`.
3. Modify `DockerBrowserService.Start` to accept resource limit parameters from config.
4. Update `cmd/scraper/main.go` to instantiate `BrowserRuntimeManager` and pass it to the server instead of `DockerBrowserService` directly.
5. Update browser HTTP handlers to call `manager.Start`/`manager.Stop` instead of `service.Start`/`service.Stop`.
6. Add `GET /browser/:id/runtime` route.
7. Deploy: existing running containers become orphans detected by startup reconciliation; they are inserted with state `running` and probed immediately.
8. Rollback: revert handlers to call `DockerBrowserService` directly; `browser_containers` table is harmless if left in place (no foreign-key constraints on other tables).

## Open Questions

- Should `GET /browser/:id/runtime` require auth or be open for internal health monitoring tooling? Proposed: require existing auth middleware (same as other `/browser/` routes).
- Should the health probe failure threshold be configurable (N consecutive failures before `unhealthy`)? Proposed: hardcode 2 consecutive failures for MVP; add `CONTAINER_HEALTH_FAIL_THRESHOLD` env var if operators request tuning.
- Should resource limit config be per-account or global? Proposed: global via env vars for MVP; per-account override is a separate change.
