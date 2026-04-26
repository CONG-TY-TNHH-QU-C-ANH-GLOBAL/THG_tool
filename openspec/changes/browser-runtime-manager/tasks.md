## 1. Database Migration

- [ ] 1.1 Add `browser_containers` table migration in `internal/store/store.go`: columns `account_id INTEGER PRIMARY KEY`, `container_id TEXT`, `state TEXT NOT NULL DEFAULT 'pending'`, `restart_count INTEGER NOT NULL DEFAULT 0`, `last_health_at DATETIME`, `last_health_ok INTEGER`, `cpu_quota INTEGER NOT NULL DEFAULT 0`, `memory_limit INTEGER NOT NULL DEFAULT 0`, `created_at DATETIME NOT NULL`, `updated_at DATETIME NOT NULL`
- [ ] 1.2 Verify migration is idempotent (`CREATE TABLE IF NOT EXISTS`)
- [ ] 1.3 Add `idx_browser_containers_state` index on `(state)` for reconciliation queries

## 2. Config Variables

- [ ] 2.1 Add `CONTAINER_HEALTH_PROBE_INTERVAL` (default `10s`) to `internal/config/config.go`
- [ ] 2.2 Add `CONTAINER_HEALTH_TIMEOUT` (default `3s`) to `internal/config/config.go`; log warning and clamp if `>= CONTAINER_HEALTH_PROBE_INTERVAL`
- [ ] 2.3 Add `CONTAINER_RESTART_POLICY` (default `on-failure:3`) to `internal/config/config.go`; parse into `RestartPolicy` struct with `Mode string` and `MaxCount int`
- [ ] 2.4 Add `CONTAINER_CPU_QUOTA` (default `0`, float64) to `internal/config/config.go`
- [ ] 2.5 Add `CONTAINER_MEMORY_LIMIT` (default `0`, int64 bytes) to `internal/config/config.go`
- [ ] 2.6 Add all new vars to `.env.example` with documentation comments

## 3. Container Lifecycle Store (`internal/store/store.go` additions)

- [ ] 3.1 Implement `UpsertBrowserContainer(accountID int64, containerID, state string, cpuQuota int64, memoryLimit int64) error`: `INSERT OR REPLACE INTO browser_containers …`
- [ ] 3.2 Implement `TransitionBrowserContainer(accountID int64, fromState, toState string) (bool, error)`: `UPDATE browser_containers SET state=?, updated_at=? WHERE account_id=? AND state=?`; return `(true, nil)` if 1 row updated, `(false, nil)` if 0 rows (stale state)
- [ ] 3.3 Implement `IncrementRestartCount(accountID int64) error`: `UPDATE browser_containers SET restart_count=restart_count+1, state='pending', updated_at=? WHERE account_id=?`
- [ ] 3.4 Implement `UpdateHealthCheck(accountID int64, ok bool) error`: `UPDATE browser_containers SET last_health_at=?, last_health_ok=? WHERE account_id=?`
- [ ] 3.5 Implement `GetBrowserContainer(accountID int64) (*BrowserContainer, error)` for status endpoint
- [ ] 3.6 Implement `ListBrowserContainersInState(states []string) ([]BrowserContainer, error)` for reconciliation

## 4. Health Probe (`internal/browser/health_probe.go`)

- [ ] 4.1 Define `ContainerHealthProbe` struct: `mu sync.Mutex`, `probes map[int64]context.CancelFunc`, `store *store.Store`, `interval time.Duration`, `timeout time.Duration`, `failThreshold int`, `manager *BrowserRuntimeManager`
- [ ] 4.2 Implement `ContainerHealthProbe.Start(accountID int64, cdpPort int)`: cancel existing probe for `accountID` if present; create new context; spawn goroutine with `runProbe(ctx, accountID, cdpPort)`
- [ ] 4.3 Implement `ContainerHealthProbe.Stop(accountID int64)`: look up and call cancel func; remove from map
- [ ] 4.4 Implement `runProbe(ctx, accountID, cdpPort)`: sleep grace period (`interval * 2`); loop — HTTP GET with `timeout` context to `http://localhost:<cdpPort>/json/version`; on success reset fail counter and call `store.UpdateHealthCheck(accountID, true)`; on failure increment counter, call `store.UpdateHealthCheck(accountID, false)`; if consecutive failures >= `failThreshold` call `manager.MarkUnhealthy(accountID)` and return; sleep `interval`

## 5. Restart Policy (`internal/browser/restart_policy.go`)

- [ ] 5.1 Define `RestartPolicy` struct: `Mode string` (`"never"`, `"on-failure"`, `"always"`), `MaxCount int`
- [ ] 5.2 Implement `ParseRestartPolicy(s string) RestartPolicy`: parse `"on-failure:3"` → `{Mode:"on-failure", MaxCount:3}`; `"always"` → `{Mode:"always"}`; `"never"` → `{Mode:"never"}`; log warning and default to `never` on invalid input
- [ ] 5.3 Implement `RestartPolicy.ShouldRestart(restartCount int) bool`: `never` → false; `always` → true; `on-failure` → `restartCount < MaxCount`

## 6. Runtime Manager (`internal/browser/runtime_manager.go`)

- [ ] 6.1 Define `BrowserRuntimeManager` struct: `service BrowserServicer`, `store *store.Store`, `probe *ContainerHealthProbe`, `policy RestartPolicy`, `jobScheduler *jobs.Scheduler`, `cfg config.Config`, `mu sync.Mutex`
- [ ] 6.2 Implement `BrowserRuntimeManager.Start(ctx context.Context) error`: call `reconcile(ctx)`; return nil
- [ ] 6.3 Implement `BrowserRuntimeManager.StartContainer(accountID, orgID int64) (*StartResult, error)`: upsert `browser_containers` row with `state='pending'`; delegate to warm pool or job scheduler per existing flow; on direct start, drive FSM through `creating → starting → running`; start health probe on success
- [ ] 6.4 Implement `BrowserRuntimeManager.StopContainer(accountID, orgID int64) error`: org ownership check; transition `running → stopping`; stop health probe; call `service.Stop`; transition `stopping → stopped`; call `store.Complete(jobID)` to mark scheduler job done; release org semaphore; mark `stopped → removed`
- [ ] 6.5 Implement `BrowserRuntimeManager.MarkUnhealthy(accountID int64)`: `TransitionBrowserContainer(running → unhealthy)`; evaluate restart policy; if restart: `IncrementRestartCount`, submit `browser_start` job; else log and leave in `stopped` after `stopping → stopped`
- [ ] 6.6 Implement `BrowserRuntimeManager.GetRuntime(accountID int64) (*BrowserContainer, error)`: call `store.GetBrowserContainer`
- [ ] 6.7 Implement `reconcile(ctx context.Context)`: list all `browser_containers` rows; list all Docker containers matching `thg-browser-*`; apply corrections per design doc (missing Docker container → removed + restart eval; orphan Docker container → insert running row + start probe; exited Docker container → stopped + restart eval)
- [ ] 6.8 Apply CPU quota and memory limit from config when calling `service.Start`: pass `NanoCPUs = cfg.ContainerCPUQuota * 1e9` and `Memory = cfg.ContainerMemoryLimit`; store values in `browser_containers` row

## 7. HTTP Handler Updates

- [ ] 7.1 In `POST /browser/start` handler: replace direct `service.Start` call with `manager.StartContainer(accountID, orgID)`; return HTTP 200 on sync start (pool hit / already running), HTTP 202 on async queue
- [ ] 7.2 In `POST /browser/stop` handler: replace direct `service.Stop` call with `manager.StopContainer(accountID, orgID)`; return HTTP 404 when no running container exists
- [ ] 7.3 Add `GET /browser/:id/runtime` handler: call `manager.GetRuntime(accountID)`; return 200 with `BrowserContainer` JSON or 404; apply existing auth middleware
- [ ] 7.4 Register `GET /browser/:id/runtime` route in `internal/server/api.go`

## 8. Wiring in `main.go`

- [ ] 8.1 Instantiate `ContainerHealthProbe` with config values
- [ ] 8.2 Instantiate `BrowserRuntimeManager` with `DockerBrowserService`, store, health probe, restart policy (parsed from config), and job scheduler
- [ ] 8.3 Call `manager.Start(ctx)` after DB migrations and job scheduler start; this triggers reconciliation
- [ ] 8.4 Pass `manager` to `Server` instead of `DockerBrowserService` for all browser start/stop operations

## 9. Verification

- [ ] 9.1 `go build ./cmd/scraper/` passes with no new warnings
- [ ] 9.2 `go test ./internal/browser/...` — unit tests: FSM transition acceptance/rejection, `ParseRestartPolicy`, `RestartPolicy.ShouldRestart` (never/on-failure/always), health probe stops on cancel, reconciliation inserts orphan row
- [ ] 9.3 Manual test: start a container via `POST /browser/start`; call `GET /browser/:id/runtime` — confirm `state=running` and `last_health_ok=1` after the grace period
- [ ] 9.4 Manual test: kill the Docker container out-of-band (not via API); wait for health probe to detect failure after 2 intervals + grace; confirm `state=unhealthy` in `GET /browser/:id/runtime` and a new `browser_start` job appears in `GET /api/v1/jobs`
- [ ] 9.5 Manual test: set `CONTAINER_RESTART_POLICY=on-failure:2`; kill container twice; confirm third kill does NOT create a new job; `restart_count=2` in runtime endpoint
- [ ] 9.6 Manual test: set `CONTAINER_CPU_QUOTA=0.5` and `CONTAINER_MEMORY_LIMIT=536870912` (512MB); start container; confirm Docker inspect shows `NanoCPUs=500000000` and `Memory=536870912`
- [ ] 9.7 Manual test: restart service while containers are running; confirm reconciliation picks up orphan containers and probes them without re-creating
