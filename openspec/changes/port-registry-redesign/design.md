## Context

The current `PortRegistry` in `internal/browser/port_registry.go` (designed in `docker-browser-service`) works as follows:

```go
type PortRegistry struct {
    mu        sync.Mutex
    allocated map[int]bool   // cdp and vnc share same map
    cdpRange  [2]int
    vncRange  [2]int
    maxConcurrent int
}

func (r *PortRegistry) Acquire() (cdpPort, vncPort int, err error) {
    r.mu.Lock()
    defer r.mu.Unlock()
    // linear scan cdpRange[0]..cdpRange[1]
    // linear scan vncRange[0]..vncRange[1]
    // brief TCP bind test per candidate
}
```

Problems at scale:
1. **Single global lock**: 1000 concurrent `Acquire()` calls serialize completely. The lock is held during the TCP bind test (syscall), making it even slower.
2. **Linear scan**: O(range_size) per call. With a 100-port range this is 100 iterations per acquire under the lock.
3. **No lease expiry**: Crashed containers leave ports allocated forever.
4. **Shared map for CDP and VNC**: Scanning one type scans through entries for the other, wasting iterations.
5. **Cap mixed into registry**: Concurrency cap checked inside the registry rather than at the scheduler level — wrong abstraction boundary.

The redesign must preserve the `Acquire(ctx) (cdpPort, vncPort int, err error)` and `Release(cdpPort, vncPort int)` call signatures so no callers need to change.

## Goals / Non-Goals

**Goals:**
- `Acquire()` is O(1) amortized with no global sequential scan.
- No double allocation under any concurrent access pattern.
- Leases expire automatically after `PORT_LEASE_TTL`; expired ports are reclaimed without a restart.
- CDP and VNC ports are allocated from fully separate pools with independent state.
- A `PortRegistryBackend` interface abstracts storage so `MemoryBackend` and `RedisBackend` are interchangeable.
- `GET /browser/ports/leases` returns all live leases with TTL remaining.
- Concurrency cap is NOT enforced by the registry (responsibility moved to scheduler semaphore).

**Non-Goals:**
- Multi-node coordination without the Redis backend.
- Port range hot-reload (changing ranges requires restart).
- TCP bind verification in the hot path — moved to startup validation only.
- Persistent lease storage for `MemoryBackend` (leases lost on restart; Docker reconciliation handles recovery).

## Decisions

### 1. Atomic bitset with CAS for lock-free allocation

**Decision**: Each pool (`CdpPool`, `VncPool`) holds an `[]uint64` bitset, one bit per port in the range. `Acquire()` on a pool does a CAS loop:
```
for each word in bitset:
    if word != allOnes:
        bit = trailingZeros(^word)   // first free bit
        if CAS(&word, old, old | (1<<bit)): return rangeStart + wordIndex*64 + bit
```
This is O(range_size / 64) in the worst case but O(1) amortized when ports are spread across words, and **requires no lock**.

**Why**: CAS loops scale linearly with goroutine count only when there is heavy contention on the same word. With a 100-port range and reasonable concurrency (< 100 concurrent acquires), contention per word is low. The approach is standard in kernel memory allocators and pool implementations.

**Alternative considered**: Per-port `sync.Mutex` array — too many objects, cache-unfriendly. Channel-based free list — O(1) but requires pre-populating and GC pressure. We chose bitset for memory efficiency and CPU cache locality.

### 2. Lease table in a separate struct, not embedded in the bitset

**Decision**: `LeaseTable` is a separate `sync.Map[port → Lease{expiresAt, accountID}]`. The bitset tracks allocation (set = allocated), and the lease table tracks metadata. `Release()` clears the bitset bit AND deletes the lease entry. The reaper scans only the lease table (not the bitset) to find expired entries.

**Why**: The bitset is optimized for fast set/clear; a struct map is optimized for iteration with metadata. Mixing the two would require locking or complex atomic structs. Separating them keeps each structure simple and independently lockable.

**Alternative considered**: Embedding lease expiry in the bitset using two bits per port (allocated + expired) — too complex for marginal gain.

### 3. Reaper goroutine on a ticker, scanning the lease table

**Decision**: A single goroutine ticks every `PORT_LEASE_REAPER_INTERVAL` (default 5m). For each lease where `time.Now().After(lease.expiresAt)`, it clears the bitset bit and deletes the lease entry, then logs the recovery with account ID and port.

**Why**: The lease table is a `sync.Map` so iteration is safe without additional locking. Reaper runs infrequently enough that scan cost is negligible. On a typical deployment the lease table has < 200 entries (one per running container).

**Alternative considered**: Per-lease `time.AfterFunc` timers — O(n) timer objects, complex cancellation when a lease is explicitly released before expiry. Ticker wins for simplicity.

### 4. `PortRegistryBackend` interface with `MemoryBackend` default

**Decision**:
```go
type PortRegistryBackend interface {
    Acquire(ctx context.Context, pool PortType) (port int, leaseID string, err error)
    Release(ctx context.Context, leaseID string) error
    Renew(ctx context.Context, leaseID string, ttl time.Duration) error
    ListLeases(ctx context.Context) ([]LeaseInfo, error)
}
```
`PortRegistry` wraps two backend instances (one CDP, one VNC). `MemoryBackend` uses the atomic bitset + lease table. `RedisBackend` uses `SET NX PX` for atomic allocation and `EXPIRE` for renewal — a standard Redis distributed lock pattern.

**Why**: The interface boundary makes the memory and Redis implementations independently testable. The caller (`PortRegistry`) never knows which backend is active. Adding a third backend (e.g., etcd) requires only a new struct implementing the interface.

**Alternative considered**: Build-tag-based conditional compilation without an interface — rejected because it prevents testing both backends in the same binary and makes the boundary harder to reason about.

### 5. TCP bind verification moved out of the hot Acquire path

**Decision**: At startup, `PortRegistry` runs a one-time validation: bind each port in both ranges and immediately release, confirming the OS will accept them. During runtime, `Acquire()` does NOT bind-test candidates — it relies on the bitset to prevent double allocation.

**Why**: The original bind test was a syscall inside the lock, adding ~0.1ms per allocation attempt. Under concurrency this compounds. The bind test was originally a defense against external processes occupying ports in the range — a condition that should be caught at startup (bad config) not at runtime (operational hazard). If an external process steals a port after startup, Docker will fail to bind and return an error, which is surfaced through the normal container start error path.

**Alternative considered**: Keep bind test in Acquire but outside the lock (test then CAS) — introduces TOCTOU: another goroutine or external process could claim the port between test and CAS. Startup-only validation is simpler and sufficient.

### 6. Lease renewal tied to container heartbeat, not automatic

**Decision**: `BrowserService` (or the warm pool) calls `PortRegistry.Renew(leaseID, ttl)` on a periodic heartbeat (every `PORT_LEASE_TTL / 2`). Heartbeat is a separate goroutine per running container. If the container crashes, the heartbeat stops and the lease expires naturally.

**Why**: The reaper cannot know if a container is running without querying Docker — that couples the registry to Docker. Instead, the heartbeat pattern decouples them: alive containers renew; dead containers don't. The reaper only needs to check timestamps.

**Alternative considered**: Reaper queries Docker to verify container liveness — adds Docker SDK dependency to the registry, breaks the single-responsibility principle.

## Risks / Trade-offs

- **CAS loop starvation under extreme contention** → Mitigation: with 100 ports and 100 goroutines all trying to acquire simultaneously, at most 1 CAS per word fails per attempt. Starvation is theoretically possible but practically requires sustained load exceeding port pool size — at which point all ports are allocated anyway and `Acquire` returns an error fast.
- **Lease TTL shorter than container lifetime causes false reclamation** → Mitigation: `PORT_LEASE_TTL` defaults to 30m, far above typical session length; heartbeat renews at TTL/2; document that TTL must exceed expected session duration + reaper interval.
- **RedisBackend split-brain if Redis restarts** → Mitigation: on Redis reconnect, the backend re-registers all leases held by running containers (reconciliation via Docker label scan). Documented as an ops procedure. Single-node deployments use `MemoryBackend` and are unaffected.
- **Startup port validation slows service start with large ranges** → Mitigation: validation is parallel (goroutine per port) with a 1s timeout per bind; on failure, only the failing port is logged (not the whole range).
- **`sync.Map` lease table iteration performance** → `sync.Map` is optimized for read-heavy workloads; the reaper is the only writer during scans. At < 200 entries, scan time is microseconds.

## Migration Plan

1. Implement `MemoryBackend` with atomic bitset + lease table in `internal/browser/port_backend.go`.
2. Implement `LeaseTable` and reaper in `internal/browser/port_lease.go`.
3. Rewrite `PortRegistry` in `internal/browser/port_registry.go` wrapping two `PortRegistryBackend` instances.
4. Add `RedisBackend` in `internal/browser/port_backend_redis.go` (behind `//go:build redis` tag).
5. Add heartbeat goroutine to `DockerBrowserService.Start()` and `WarmPool.startSlot()`.
6. Add `GET /browser/ports/leases` handler.
7. Update `cmd/scraper/main.go` to select backend from `PORT_REGISTRY_BACKEND` env var.
8. Deploy: stop service → redeploy → start. In-flight containers from prior deploy have no leases; they will be recovered by Docker reconciliation on startup (existing mechanism from `docker-browser-service`).
9. Rollback: revert `port_registry.go` to prior implementation — call sites are unchanged.

## Open Questions

- Should `Renew()` be called by `BrowserService` or should the `PortRegistry` itself spawn a renewal goroutine per lease? Current design: caller owns the heartbeat. Simpler, but risks forgotten renewals if a new `BrowserService` implementation forgets to call it.
- Should the startup port validation run in parallel or sequentially? Proposed: parallel with bounded goroutine count (32 at a time) for large ranges.
- What is the correct `PORT_LEASE_TTL` for warm pool containers, which may sit idle for `WARM_IDLE_TIMEOUT` (10m default)? TTL must exceed idle timeout. Proposed: document that `PORT_LEASE_TTL > WARM_IDLE_TIMEOUT` is a required invariant.
