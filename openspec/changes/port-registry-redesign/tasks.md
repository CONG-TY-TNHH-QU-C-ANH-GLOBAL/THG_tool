## 1. Config and Environment

- [ ] 1.1 Add `PORT_REGISTRY_BACKEND` (default `memory`), `PORT_LEASE_TTL` (default `30m`), `PORT_LEASE_REAPER_INTERVAL` (default `5m`), and `REDIS_ADDR` to `internal/config/config.go`
- [ ] 1.2 Add all four env vars to `.env.example` with documented defaults and a note that `PORT_LEASE_TTL > WARM_IDLE_TIMEOUT` is a required invariant

## 2. Lease Table

- [ ] 2.1 Create `internal/browser/port_lease.go` — define `Lease` struct (`LeaseID string`, `AccountID int64`, `CDPPort int`, `VNCPort int`, `ExpiresAt time.Time`) and `LeaseTable` struct wrapping `sync.Map[leaseID → *Lease]`
- [ ] 2.2 Implement `LeaseTable.Add(lease *Lease)`, `LeaseTable.Get(leaseID) (*Lease, bool)`, `LeaseTable.Delete(leaseID)`, `LeaseTable.Renew(leaseID, ttl) error`
- [ ] 2.3 Implement `LeaseTable.Expired() []*Lease` — returns all leases where `time.Now().After(lease.ExpiresAt)` (used by reaper)
- [ ] 2.4 Implement `LeaseTable.List() []LeaseInfo` — snapshot of all leases with `ExpiresInSeconds` calculated at call time
- [ ] 2.5 Write unit tests: Add+Get, Delete removes entry, Renew extends expiry, Renew unknown returns error, Expired returns only past-expiry entries

## 3. PortRegistryBackend Interface

- [ ] 3.1 Create `internal/browser/port_backend.go` — define `PortType` enum (`CDP`, `VNC`), `PortRegistryBackend` interface with `Acquire(ctx, pool PortType) (port int, leaseID string, err error)`, `Release(ctx, leaseID string) error`, `Renew(ctx, leaseID string, ttl time.Duration) error`, `ListLeases(ctx) ([]LeaseInfo, error)`

## 4. MemoryBackend — Atomic Bitset

- [ ] 4.1 Implement `MemoryBackend` struct in `internal/browser/port_backend.go` with `cdpBits []uint64`, `vncBits []uint64`, `leases *LeaseTable`, `cdpStart int`, `vncStart int`
- [ ] 4.2 Implement `NewMemoryBackend(cdpRange, vncRange [2]int) (*MemoryBackend, error)` — allocate bitset slices sized `ceil(rangeSize/64)`, run startup port bindability validation (parallel, warn on conflicts, mark conflicting ports as allocated)
- [ ] 4.3 Implement `MemoryBackend.acquireFromPool(bits []uint64, rangeStart int) (port int, bitIndex int, err error)` — CAS loop: find first zero bit via `math/bits.TrailingZeros64(^word)`, attempt `atomic.CompareAndSwapUint64`; retry on collision; return error if all words are `^uint64(0)`
- [ ] 4.4 Implement `MemoryBackend.Acquire(ctx, pool PortType) (port int, leaseID string, err error)` — call `acquireFromPool` for the selected pool, generate UUID lease ID, add to `LeaseTable`; on error clear any acquired port
- [ ] 4.5 Implement `MemoryBackend.Release(ctx, leaseID string) error` — look up lease, clear the bit for the port via `atomic.AndUint64`, delete lease entry
- [ ] 4.6 Implement `MemoryBackend.Renew` and `ListLeases` delegating to `LeaseTable`
- [ ] 4.7 Write unit tests: 100 concurrent Acquire calls on 100-port range → no duplicates, all ports distinct; exhausted pool returns error fast; Release clears bit; CAS retry counted via metrics (optional)

## 5. RedisBackend

- [ ] 5.1 Create `internal/browser/port_backend_redis.go` with build tag `//go:build redis`
- [ ] 5.2 Implement `RedisBackend` struct using `github.com/redis/go-redis/v9` client; key format: `port:cdp:{port}` and `port:vnc:{port}`
- [ ] 5.3 Implement `RedisBackend.Acquire(ctx, pool PortType)` — iterate range, attempt `SET key leaseJSON NX PX ttlMs`; first successful SET wins; return port and lease ID
- [ ] 5.4 Implement `RedisBackend.Release(ctx, leaseID)` — `DEL port:cdp:{port}` and `port:vnc:{port}` using lease metadata
- [ ] 5.5 Implement `RedisBackend.Renew(ctx, leaseID, ttl)` — `EXPIRE key ttlSeconds` for both keys
- [ ] 5.6 Implement `RedisBackend.ListLeases(ctx)` — `SCAN` with pattern `port:*`, parse values into `LeaseInfo` slice
- [ ] 5.7 Write integration tests (skipped unless `REDIS_ADDR` env var set): concurrent Acquire across two backend instances → no duplicates; key TTL expiry reclaims port

## 6. Reaper Goroutine

- [ ] 6.1 Add `startReaper(ctx context.Context)` method to `PortRegistry` — ticker at `PORT_LEASE_REAPER_INTERVAL`; on tick call `backend.ListLeases`, filter expired, call `backend.Release` for each; log at WARN with account ID, port pair, and how long overdue
- [ ] 6.2 Wire reaper start into `PortRegistry.Start(ctx)` method; wire stop via context cancellation
- [ ] 6.3 Write unit test: after TTL elapses on a MemoryBackend lease without Renew, the bit is cleared and the port is acquirable again (use a very short TTL + reaper interval in the test)

## 7. Updated PortRegistry

- [ ] 7.1 Rewrite `internal/browser/port_registry.go` — `PortRegistry` holds two `PortRegistryBackend` instances (cdp, vnc); `Acquire(ctx) (cdpPort, vncPort int, leaseID string, err error)`; `Release(ctx, leaseID string) error`; `Renew(ctx, leaseID string) error`; `ListLeases(ctx) ([]LeaseInfo, error)`
- [ ] 7.2 Remove `maxConcurrent` field and cap enforcement from `PortRegistry` entirely
- [ ] 7.3 Update `NewPortRegistry(cfg Config) (*PortRegistry, error)` to select backend from `PORT_REGISTRY_BACKEND` config

## 8. Heartbeat Integration

- [ ] 8.1 In `DockerBrowserService.Start()`, after container is running, launch a goroutine that calls `PortRegistry.Renew(leaseID)` every `PORT_LEASE_TTL / 2`; goroutine exits when the container's context is cancelled
- [ ] 8.2 In `WarmPool.startSlot()`, similarly launch a renewal goroutine per warm container; goroutine exits when the slot is claimed or recycled
- [ ] 8.3 Ensure `PortRegistry.Release` is always called in both `DockerBrowserService.Stop()` and the warm pool eviction path; pass `leaseID` stored in `ContainerInfo` / `WarmSlot`

## 9. Updated Call Sites

- [ ] 9.1 Update `ContainerInfo` struct to include `LeaseID string` field
- [ ] 9.2 Update `WarmSlot` struct to include `LeaseID string` field
- [ ] 9.3 Update all `PortRegistry.Acquire()` call sites (DockerBrowserService, WarmPool) to receive and store `leaseID`
- [ ] 9.4 Update all `PortRegistry.Release()` call sites to pass `leaseID` instead of individual port numbers

## 10. REST Handler and Route

- [ ] 10.1 Add `GET /browser/ports/leases` handler in `internal/server/browser_handlers.go` — call `PortRegistry.ListLeases(ctx)`, return JSON array
- [ ] 10.2 Register the route in `internal/server/api.go`

## 11. Verification

- [ ] 11.1 `go build ./cmd/scraper/` passes (without `redis` tag); `go build -tags redis ./cmd/scraper/` also passes
- [ ] 11.2 Run unit tests: `go test ./internal/browser/...` — all new and existing tests pass
- [ ] 11.3 Concurrent allocation stress test: `go test -run TestMemoryBackendConcurrent -race -count=5 ./internal/browser/...` — race detector reports no issues
- [ ] 11.4 Manual lease expiry test: set `PORT_LEASE_TTL=1m` and `PORT_LEASE_REAPER_INTERVAL=30s`; kill a running container without calling Stop; confirm port reappears in `GET /browser/ports/leases` as expired within 90s
- [ ] 11.5 Verify `GET /browser/ports/leases` returns correct `expires_in_seconds` for all active containers
