## Why

The `docker-browser-service` `PortRegistry` uses a sequential scan over a `map[int]bool` under a single mutex: under 1000 concurrent allocation requests, all goroutines serialize on the lock and the scan degrades to O(n) per call. Worse, there is no expiry on allocated ports — a crashed container leaves its ports permanently leaked until the service restarts. As the system scales toward hundreds of concurrent browsers and eventually multi-node deployment, the registry must be replaced with an allocation strategy that is O(1), lease-based, and optionally distributed.

## What Changes

- Replace the scan-based `map[int]bool` allocator with a **bitset-per-pool** design: two separate pools (CDP, VNC), each using an atomic uint64 array as a bitset. `Acquire()` is a CAS loop over the bitset — no linear scan, no global lock.
- Introduce **lease-based allocation**: each acquired port pair is tagged with an expiry timestamp (`PORT_LEASE_TTL`, default 30m). A background goroutine reclaims expired leases automatically, recovering ports from crashed containers without a service restart.
- Add a **`PortRegistryBackend` interface** with two implementations: `MemoryBackend` (current single-node) and `RedisBackend` (future multi-node). The active backend is selected via `PORT_REGISTRY_BACKEND=memory|redis`.
- Remove the concurrency cap from `PortRegistry` — cap enforcement stays in the scheduler semaphore (correct separation of concerns). The registry's only job is port uniqueness.
- Add `GET /browser/ports/leases` endpoint for ops visibility of all active leases and their remaining TTL.

## Capabilities

### New Capabilities

- `port-lease-management`: Lease-based port allocation with configurable TTL, automatic expiry via background reaper, and explicit renewal on container heartbeat.
- `port-registry-backend`: Pluggable backend interface (`memory` or `redis`) with identical `Acquire`/`Release`/`Renew`/`List` semantics, enabling single-node and multi-node deployment from the same codebase.

### Modified Capabilities

- `browser-port-registry`: Core allocation changes from mutex scan to atomic bitset CAS; concurrency cap removed (delegated to scheduler); lease TTL added to every allocation; separate port pools per type. Requires delta spec.

## Impact

- **Code**: `internal/browser/port_registry.go` rewritten; new `internal/browser/port_lease.go`; new `internal/browser/port_backend.go` (interface + memory impl); new `internal/browser/port_backend_redis.go` (Redis impl, build-tag optional); new handler `GET /browser/ports/leases` in `browser_handlers.go`.
- **APIs**: New `GET /browser/ports/leases` endpoint. Existing `Acquire()`/`Release()` call sites unchanged at the Go API level (signature compatible).
- **Dependencies**: `RedisBackend` requires `github.com/redis/go-redis/v9` — added as an optional dependency (build tag `redis`); `MemoryBackend` has no new deps.
- **Config**: New env vars `PORT_REGISTRY_BACKEND` (default `memory`), `PORT_LEASE_TTL` (default `30m`), `PORT_LEASE_REAPER_INTERVAL` (default `5m`), `REDIS_ADDR` (required only when backend=redis).
- **Warm pool and scheduler**: Both call `PortRegistry.Acquire()` and `Release()` — no call-site changes required; lease renewal is handled transparently by the registry on each heartbeat tick.
