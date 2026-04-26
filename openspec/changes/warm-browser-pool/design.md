## Context

After `docker-browser-service` and `browser-scheduler`, the flow is:

```
POST /browser/start → Scheduler.Submit() → job queue → worker → DockerBrowserService.Start() → 2–4s wait
```

The bottleneck is the container initialization time: Docker must pull layers (first run), create the overlay filesystem, start Xvfb, start x11vnc, and launch Chrome — all serially. This cannot be eliminated from the cold path.

The warm pool shifts this work to idle time: containers are started when no request is waiting, so the slot is ready before the account owner asks for it. The system is a Facebook automation tool where accounts are finite and known in advance (staff accounts, not anonymous users), making per-account pre-warming viable.

Constraints from existing infrastructure:
- `PortRegistry` already tracks allocated ports — warm containers must acquire ports through it.
- `MAX_CONCURRENT_BROWSERS` caps total live containers (warm + running). Pool size must be within this cap.
- The scheduler's semaphore tracks running containers — warm containers must hold semaphore slots until released by `Stop`.

## Goals / Non-Goals

**Goals:**
- On a pool hit, `POST /browser/start` returns container info synchronously (HTTP 200, <100ms).
- Pool replenishes asynchronously after each assignment — next request for same account is also fast.
- Idle containers exceeding `WARM_IDLE_TIMEOUT` are recycled to avoid Chrome session rot and memory buildup.
- Pool size is bounded by `WARM_POOL_SIZE` and within `MAX_CONCURRENT_BROWSERS`.
- Pool accounts are configurable (`WARM_POOL_ACCOUNTS`); defaults to all DB-active accounts up to `WARM_POOL_SIZE`.
- `GET /browser/pool/status` exposes current pool state (which accounts are warmed, idle durations).

**Non-Goals:**
- Anonymous/unassigned warm containers — each slot is pre-assigned to a specific `account_id`.
- Warm pool persistence across service restarts — pool rebuilds from scratch on startup.
- Per-account multiple warm slots — one warm slot per account maximum.
- Kubernetes or multi-node pool coordination.

## Decisions

### 1. Pool is a `map[accountID → *WarmSlot]` guarded by a mutex, not a channel

**Decision**: `WarmPool` holds `slots map[int64]*WarmSlot` under `sync.RWMutex`. A `WarmSlot` stores the pre-started `ContainerInfo` (port pair, container ID) and `idleAt time.Time`.

**Why**: Warm containers are keyed by `account_id` — lookup must be O(1) by account. A channel would require scanning to find the right account. A map with a mutex is the natural fit.

**Alternative considered**: `sync.Map` — rejected because iteration (needed for the recycler) is awkward and `RWMutex` + regular map performs better under the read-heavy pool-hit path.

### 2. Warm containers hold a `PortRegistry` allocation AND a scheduler semaphore slot

**Decision**: When the warm pool starts a container, it calls `PortRegistry.Acquire()` and acquires one slot from the scheduler's semaphore channel. The slot is held until the container is stopped (same as a running container).

**Why**: This ensures warm containers are counted against `MAX_CONCURRENT_BROWSERS`. Without semaphore holding, the pool could silently exceed the node's capacity. The existing scheduler + Docker service already rely on this accounting.

**Consequence**: `WARM_POOL_SIZE` must be ≤ `MAX_CONCURRENT_BROWSERS`. At startup, the pool tries to fill up to `min(WARM_POOL_SIZE, available_semaphore_slots)`.

**Alternative considered**: Let warm containers bypass the semaphore and only acquire it on assignment — rejected because it breaks the invariant that the semaphore accurately reflects all live containers.

### 3. Replenishment runs in a single background goroutine per pool event

**Decision**: When a warm slot is claimed or recycled, the `WarmPool` sends the `account_id` to a `replenishCh chan int64`. A single `replenishLoop` goroutine drains this channel and calls `DockerBrowserService.Start` for each account. This ensures at most one concurrent replenishment per account.

**Why**: Avoids goroutine storms if many accounts are assigned simultaneously. The channel is buffered (capacity = `WARM_POOL_SIZE`) so senders never block.

**Alternative considered**: Spawn a goroutine per replenishment event — simpler code but no back-pressure; under burst assignment all accounts replenish simultaneously, potentially overwhelming Docker.

### 4. Recycler runs on a ticker, not per-slot timers

**Decision**: A single goroutine ticks every `WARM_IDLE_TIMEOUT / 2` and scans the pool for slots where `time.Since(slot.idleAt) > WARM_IDLE_TIMEOUT`. Expired slots are stopped and added to the replenish queue.

**Why**: Per-slot `time.AfterFunc` creates one timer per warm container and requires careful cancellation when a slot is claimed. A ticker scan is simpler and the imprecision (up to `WARM_IDLE_TIMEOUT / 2` extra idle time) is acceptable.

**Alternative considered**: A min-heap of expiry times for O(log n) next-expiry lookup — unnecessary overhead for pool sizes of 3–20 slots.

### 5. Pool hit returns HTTP 200 with immediate container info; pool miss falls through to scheduler (HTTP 202)

**Decision**: `browser_handlers.go` calls `WarmPool.Claim(accountID)` first. On hit: return `{ status: "running", cdp_port, vnc_port, container_id }` synchronously. On miss: call `Scheduler.Submit()` and return job reference as before.

**Why**: HTTP 200 vs 202 distinction lets the frontend differentiate pool hits (show browser immediately) from cold starts (show "queued" pill). Clients already handle both codes from the `browser-scheduler` change.

**Alternative considered**: Always return 202 and resolve instantly for pool hits — rejected because it forces an unnecessary polling round-trip when the result is already available.

### 6. Profile isolation via per-account profile directory (unchanged from docker-browser-service)

**Decision**: Warm containers mount `data/profiles/account_{id}/` at startup, identical to cold-start containers. No profile copying or sharing between warm slots.

**Why**: Each slot is pre-assigned to one account, so the correct profile can be mounted at container creation time. This is safe and consistent with the existing profile isolation model.

**Alternative considered**: Anonymous containers with profile inject on assignment — requires Docker volume hot-swap (not supported) or mounting an empty dir then copying files into the running container via `docker cp` (slow, negates pool benefit).

## Risks / Trade-offs

- **Warm containers consume capacity even when not requested** → Mitigation: `WARM_POOL_SIZE` defaults to 3 (small); document that it must be ≤ `MAX_CONCURRENT_BROWSERS - expected_concurrent_manual_sessions`.
- **Pool account list stale after DB account changes** → Mitigation: pool re-reads active accounts from DB on each recycler tick; newly added accounts enter the pool on the next tick; deleted accounts' slots are evicted.
- **Chrome session in warm container accumulates state (cookies, open tabs) before assignment** → Mitigation: warm containers start Chrome without restoring previous session (`--no-first-run --no-startup-window`); profile data is on disk and loaded normally when account owner navigates to Facebook.
- **Replenish goroutine blocked by Docker slowness causes pool depletion** → Mitigation: replenishment is best-effort; pool depletes gracefully to the scheduler path. Monitor via `GET /browser/pool/status`.
- **Double assignment race (two requests claim same slot)** → Mitigation: `WarmPool.Claim()` uses a mutex-guarded delete from the map — `delete` is atomic under the lock; the slot is removed before the response is sent, preventing double-claim.

## Migration Plan

1. Implement `internal/browser/warm_pool.go` with `WarmPool`, `WarmSlot`, replenish loop, recycler loop.
2. Update `internal/server/browser_handlers.go`: check `WarmPool.Claim()` before `Scheduler.Submit()`.
3. Add `GET /browser/pool/status` handler and route in `api.go`.
4. Update `cmd/scraper/main.go`: instantiate `WarmPool`, pass scheduler semaphore reference, call `WarmPool.Start()` after scheduler start.
5. Add env vars to `config.go` and `.env.example`.
6. Deploy: no migration needed. Pool fills within `WARM_IDLE_TIMEOUT` of startup.
7. Rollback: remove `WarmPool.Claim()` call in handler — falls back to scheduler path with no data loss. Warm containers are stopped by reconciliation on next startup.

## Open Questions

- Should the pool pre-warm ALL active accounts or only a fixed `WARM_POOL_SIZE` subset? Current design: fill up to `WARM_POOL_SIZE` slots using the N most-recently-active accounts. Ordering heuristic TBD.
- Should `WARM_POOL_ACCOUNTS` accept a whitelist override? Proposed: yes, a comma-separated list overrides the auto-selection.
- What happens when `WARM_POOL_SIZE > MAX_CONCURRENT_BROWSERS`? Proposed: log a warning at startup and cap pool size to `MAX_CONCURRENT_BROWSERS`.
