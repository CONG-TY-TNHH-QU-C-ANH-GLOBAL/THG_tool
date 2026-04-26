## ADDED Requirements

### Requirement: Pluggable backend interface
The system SHALL define a `PortRegistryBackend` interface with methods `Acquire`, `Release`, `Renew`, and `ListLeases`. All port allocation logic SHALL be expressed through this interface so that callers (`PortRegistry`) are independent of the storage implementation.

#### Scenario: Memory backend selected by default
- **WHEN** `PORT_REGISTRY_BACKEND` is not set or is set to `memory`
- **THEN** the service uses `MemoryBackend` for port allocation with no external dependencies

#### Scenario: Redis backend selected via config
- **WHEN** `PORT_REGISTRY_BACKEND=redis` is set
- **THEN** the service uses `RedisBackend` and connects to `REDIS_ADDR` at startup; failure to connect causes the service to fail with a clear error

#### Scenario: Invalid backend value causes startup failure
- **WHEN** `PORT_REGISTRY_BACKEND=invalid` is set
- **THEN** the service fails to start with a clear configuration error naming the invalid value and listing valid options

### Requirement: MemoryBackend — atomic lock-free allocation
The system's `MemoryBackend` SHALL allocate ports using a CAS loop over an atomic bitset (one bit per port in the range) without acquiring a global mutex. Two separate bitsets SHALL be maintained: one for CDP ports and one for VNC ports.

#### Scenario: Concurrent allocations do not collide
- **WHEN** 100 goroutines call `Acquire` simultaneously on a 100-port range
- **THEN** each goroutine receives a distinct port number; no port is returned to more than one caller

#### Scenario: All bits set returns exhausted error
- **WHEN** all bits in the CDP bitset are set (all ports allocated)
- **THEN** `Acquire` returns an error immediately without blocking or looping indefinitely

#### Scenario: Cleared bit is available for reuse
- **WHEN** `Release` is called for a port
- **THEN** the corresponding bit is cleared atomically and subsequent `Acquire` calls may return that port

### Requirement: RedisBackend — distributed atomic allocation
The system's `RedisBackend` SHALL use Redis `SET key NX PX ttl` to atomically claim a port key. A claimed key SHALL contain the lease metadata. `Release` SHALL delete the key. `Renew` SHALL use `EXPIRE` to extend the TTL.

#### Scenario: Atomic Redis allocation prevents double claim
- **WHEN** two nodes call `Acquire` for the same port simultaneously
- **THEN** exactly one succeeds (SET NX wins) and the other moves to the next candidate port

#### Scenario: Redis key expiry reclaims port without reaper
- **WHEN** a lease's Redis key TTL expires (container crashed, no renewal)
- **THEN** the key is automatically deleted by Redis and the port is available for new allocations without a reaper goroutine

#### Scenario: Redis connection failure causes Acquire to return error
- **WHEN** `RedisBackend.Acquire` is called and the Redis connection is unavailable
- **THEN** the method returns an error immediately; the service does not retry indefinitely and the container start is aborted

### Requirement: Backend startup validation
Each backend SHALL validate its port ranges at startup by confirming the range is non-empty, correctly ordered, and (for `MemoryBackend`) that each port in the range is bindable on the host OS.

#### Scenario: Invalid range rejected at startup
- **WHEN** `BROWSER_CDP_PORT_RANGE=9322-9222` (inverted) is configured
- **THEN** the service fails to start with a clear error describing the invalid range

#### Scenario: Memory backend validates port bindability at startup
- **WHEN** the service starts with `MemoryBackend` and a port in the range is already in use by another process
- **THEN** a warning is logged for that port; the port is marked allocated in the bitset from startup (effectively excluded); the service continues
