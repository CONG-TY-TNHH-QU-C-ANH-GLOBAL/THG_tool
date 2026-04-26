## 1. Config and Environment

- [ ] 1.1 Add `WARM_POOL_SIZE` (default 3), `WARM_IDLE_TIMEOUT` (default 10m), and `WARM_POOL_ACCOUNTS` (optional, comma-separated) to `internal/config/config.go`
- [ ] 1.2 Add the three new env vars to `.env.example` with documented defaults and a note that `WARM_POOL_SIZE ≤ MAX_CONCURRENT_BROWSERS`

## 2. Warm Pool Core

- [ ] 2.1 Create `internal/browser/warm_pool.go` — define `WarmSlot` struct (`AccountID`, `ContainerInfo`, `idleAt time.Time`) and `WarmPool` struct (`slots map[int64]*WarmSlot`, `mu sync.RWMutex`, `replenishCh chan int64`, config fields)
- [ ] 2.2 Implement `NewWarmPool(cfg Config, svc BrowserServicer, registry *PortRegistry, semaphore chan struct{}) *WarmPool`
- [ ] 2.3 Implement `WarmPool.Claim(accountID int64) (*WarmSlot, bool)` — mutex-guarded delete from map; returns slot and true on hit, nil and false on miss
- [ ] 2.4 Implement `WarmPool.Depth() int` — returns current len(slots) under RLock
- [ ] 2.5 Implement `WarmPool.Slots() []WarmSlotInfo` — snapshot of current slots for status endpoint (account ID, container ID, ports, idle seconds)
- [ ] 2.6 Implement `WarmPool.startSlot(ctx, accountID int64) error` — acquire semaphore slot, acquire port pair, call `BrowserServicer.Start`, add to map with `idleAt = time.Now()`; release semaphore+ports on error
- [ ] 2.7 Write unit tests: Claim hit removes slot, Claim miss returns false, double-claim returns false (race), Depth accurate after claim

## 3. Replenishment Loop

- [ ] 3.1 Implement `WarmPool.replenishLoop(ctx)` goroutine — range over buffered `replenishCh`; for each account ID call `startSlot` if pool < `WARM_POOL_SIZE` and semaphore has capacity; log errors without crashing
- [ ] 3.2 Implement `WarmPool.enqueueReplenish(accountID int64)` — non-blocking send to `replenishCh`; drop if channel full (capacity = `WARM_POOL_SIZE`) and log a warning
- [ ] 3.3 Write unit tests: replenishment triggered after Claim, replenishment skipped when at capacity, channel does not block caller

## 4. Recycler Loop

- [ ] 4.1 Implement `WarmPool.recyclerLoop(ctx)` goroutine — ticker at `WARM_IDLE_TIMEOUT / 2`; on each tick scan slots for `time.Since(slot.idleAt) > WARM_IDLE_TIMEOUT`; for expired slots call `BrowserServicer.Stop`, release semaphore slot and port pair, delete from map, enqueue replenishment
- [ ] 4.2 On each recycler tick, re-read active accounts from DB; add accounts missing from pool to replenish queue (up to `WARM_POOL_SIZE`); evict slots for accounts no longer active
- [ ] 4.3 Write unit tests: expired slot is evicted and slot map shrinks, non-expired slot is not evicted, inactive account slot is evicted, new active account is enqueued for replenishment

## 5. Pool Startup and Shutdown

- [ ] 5.1 Implement `WarmPool.Start(ctx context.Context, accounts []int64)` — resolve account list (from `WARM_POOL_ACCOUNTS` config or DB query for most-recently-active), cap to `min(WARM_POOL_SIZE, available_semaphore_slots)`, log warning if capped, call `startSlot` for each, then launch `replenishLoop` and `recyclerLoop` goroutines
- [ ] 5.2 Implement `WarmPool.Stop(ctx context.Context)` — cancel context (stops goroutines), stop and remove all warm containers, release all semaphore slots and port pairs
- [ ] 5.3 Add helper `resolveWarmAccounts(ctx, db, cfg) ([]int64, error)` — parse `WARM_POOL_ACCOUNTS` if set, else query `SELECT id FROM accounts WHERE active=1 ORDER BY last_active DESC LIMIT ?`

## 6. REST Handler Integration

- [ ] 6.1 In `internal/server/browser_handlers.go`, update `POST /browser/start` handler: call `WarmPool.Claim(accountID)` first; on hit return HTTP 200 with container info and call `WarmPool.enqueueReplenish`; on miss fall through to `Scheduler.Submit()`
- [ ] 6.2 Add `GET /browser/pool/status` handler — call `WarmPool.Slots()` and `WarmPool.Depth()`, return JSON `{ pool_size, depth, capacity, slots: [...] }`
- [ ] 6.3 Register `/browser/pool/status` route in `internal/server/api.go`

## 7. Wiring in main.go

- [ ] 7.1 In `cmd/scraper/main.go`, instantiate `WarmPool` after `Scheduler` (pass scheduler's semaphore channel reference and `PortRegistry`)
- [ ] 7.2 Query active accounts from DB and call `WarmPool.Start(ctx, accounts)` after `Scheduler.Start()` and before HTTP server accepts requests
- [ ] 7.3 Add `WarmPool.Stop(ctx)` to the deferred/shutdown path before `Scheduler.Shutdown()`

## 8. Frontend Updates

- [ ] 8.1 Update `browserStartAccount()` in `app.js` to handle HTTP 200 (pool hit) directly — show running browser immediately without polling
- [ ] 8.2 Preserve existing HTTP 202 polling path for pool miss (no change needed, already implemented)
- [ ] 8.3 Add a "warm" indicator to the account status pill when `GET /browser/pool/status` lists the account as a warm slot (e.g., ⚡ icon or "Ready" label)

## 9. Verification

- [ ] 9.1 `go build ./cmd/scraper/` passes with no new warnings
- [ ] 9.2 Run unit tests: `go test ./internal/browser/...` — all new tests pass
- [ ] 9.3 Manual latency test: with `WARM_POOL_SIZE=1`, start service, wait 5s for pool to fill, call `POST /browser/start` for warmed account; confirm HTTP 200 returned in <100ms
- [ ] 9.4 Manual recycler test: set `WARM_IDLE_TIMEOUT=1m`, wait 2 minutes without claiming; confirm via `GET /browser/pool/status` that slot was evicted and replaced
- [ ] 9.5 Manual capacity test: set `WARM_POOL_SIZE=5` and `MAX_CONCURRENT_BROWSERS=2`; confirm startup logs a warning and pool depth is capped at 2
