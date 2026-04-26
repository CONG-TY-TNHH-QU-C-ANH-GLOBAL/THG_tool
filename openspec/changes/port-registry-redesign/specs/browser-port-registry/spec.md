## MODIFIED Requirements

### Requirement: Dynamic port acquisition for CDP and VNC
The system SHALL allocate a free CDP port and a free VNC port to each new browser container using atomic CAS over separate per-type bitset pools, returning a unique lease ID alongside the port numbers. A port SHALL be considered free only if its bit is clear in the pool bitset. OS-level bind verification is performed at startup only, not on each acquisition.

#### Scenario: Ports acquired successfully
- **WHEN** a new container is being started and free ports are available in both pools
- **THEN** the registry returns a unique `(cdpPort, vncPort, leaseID)` triple; the leaseID is valid for `PORT_LEASE_TTL` duration

#### Scenario: CDP pool exhausted
- **WHEN** all bits in the CDP bitset are set
- **THEN** `Acquire` returns an error immediately and no VNC port is allocated

#### Scenario: VNC pool exhausted
- **WHEN** all bits in the VNC bitset are set after a CDP port has been tentatively acquired
- **THEN** `Acquire` returns an error, the tentatively acquired CDP port is released back to the bitset, and no lease is created

#### Scenario: Concurrent allocations are collision-free
- **WHEN** multiple goroutines call `Acquire` simultaneously
- **THEN** each receives a distinct `(cdpPort, vncPort)` pair with no duplicates, without a global sequential lock

### Requirement: Port release on container stop
The system SHALL reclaim allocated ports when `Release(ctx, leaseID)` is called, clearing the corresponding bits in both pool bitsets and removing the lease entry. Ports SHALL be immediately available for new allocations after release.

#### Scenario: Ports released after stop
- **WHEN** `Release(ctx, leaseID)` is called after a container stops
- **THEN** both the CDP and VNC bits are cleared atomically and the lease entry is deleted; subsequent `Acquire` calls may return those ports

#### Scenario: Stop failure does not leak ports permanently
- **WHEN** Docker fails to remove the container but `Release` is called
- **THEN** ports are reclaimed in the registry immediately; if the container is later found by Docker reconciliation, it is stopped without affecting port state

### Requirement: Configurable port ranges
The system SHALL read port range boundaries from `BROWSER_CDP_PORT_RANGE` and `BROWSER_VNC_PORT_RANGE` env vars in `<start>-<end>` format. Ranges are validated at startup; invalid formats or inverted ranges cause immediate startup failure.

#### Scenario: Valid range configured
- **WHEN** `BROWSER_CDP_PORT_RANGE=9222-9322` is set
- **THEN** the CDP bitset covers exactly ports 9222 through 9322 (101 ports)

#### Scenario: Invalid range format causes startup failure
- **WHEN** `BROWSER_CDP_PORT_RANGE=abc` or an inverted range like `9322-9222` is set
- **THEN** the service fails to start with a clear error message identifying the invalid variable and value

#### Scenario: Default ranges used when env vars absent
- **WHEN** neither `BROWSER_CDP_PORT_RANGE` nor `BROWSER_VNC_PORT_RANGE` is set
- **THEN** the registry uses `9222-9322` for CDP and `5900-6000` for VNC

## REMOVED Requirements

### Requirement: Concurrency cap enforcement
**Reason**: Concurrency cap enforcement is the responsibility of the scheduler semaphore (`browser-scheduler` change). The registry's sole responsibility is port uniqueness. Mixing cap logic into the registry created the wrong abstraction boundary and caused the registry to need awareness of the scheduler's state.
**Migration**: No caller changes needed. Cap enforcement continues to work via the scheduler's semaphore channel of capacity `MAX_CONCURRENT_BROWSERS`. The registry no longer rejects `Acquire` calls based on count — it only rejects when ports are physically exhausted.
