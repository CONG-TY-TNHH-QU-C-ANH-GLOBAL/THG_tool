## ADDED Requirements

### Requirement: Lease issued on every port acquisition
The system SHALL associate a unique lease ID and an expiry timestamp with every acquired port pair. The lease ID SHALL be returned to the caller alongside the port numbers and MUST be used for all subsequent `Release` and `Renew` operations.

#### Scenario: Lease created on successful acquisition
- **WHEN** `Acquire(ctx)` returns successfully
- **THEN** the response includes a non-empty `leaseID` string and `expiresAt = time.Now() + PORT_LEASE_TTL`

#### Scenario: Lease not created on acquisition failure
- **WHEN** `Acquire(ctx)` returns an error (pool exhausted, range exhausted)
- **THEN** no lease entry is created in the lease table

### Requirement: Lease renewal extends expiry
The system SHALL extend the expiry of an active lease by `PORT_LEASE_TTL` from the time of renewal when `Renew(ctx, leaseID, ttl)` is called. Renewal SHALL fail if the lease ID does not exist (already expired or released).

#### Scenario: Successful renewal extends expiry
- **WHEN** `Renew(ctx, leaseID, ttl)` is called for a valid lease
- **THEN** the lease's `expiresAt` is updated to `time.Now() + ttl` and the port remains allocated

#### Scenario: Renewal of unknown lease returns error
- **WHEN** `Renew(ctx, leaseID, ttl)` is called with a lease ID not in the table
- **THEN** the system returns an error and does not modify any lease state

#### Scenario: Heartbeat renews at half TTL
- **WHEN** a container is running and `PORT_LEASE_TTL / 2` has elapsed since the last renewal
- **THEN** `BrowserService` calls `Renew` for that container's lease, ensuring the lease never expires while the container is alive

### Requirement: Automatic lease expiry and port recovery
The system SHALL run a background reaper goroutine that reclaims ports from leases whose `expiresAt` has passed. Reclaimed ports SHALL immediately become available for new allocations.

#### Scenario: Expired lease reclaimed by reaper
- **WHEN** `PORT_LEASE_REAPER_INTERVAL` has elapsed and a lease's `expiresAt` is in the past
- **THEN** the reaper clears the port's allocation bit in the pool, removes the lease entry, and logs the reclaimed port and account ID at WARN level

#### Scenario: Active lease not reclaimed
- **WHEN** the reaper ticks and a lease's `expiresAt` is in the future
- **THEN** the lease is not modified and the port remains allocated

#### Scenario: Reaper runs on configured interval
- **WHEN** `PORT_LEASE_REAPER_INTERVAL=5m` is set
- **THEN** the reaper ticks every 5 minutes; with default value the reaper ticks every 5 minutes

#### Scenario: Explicit release before expiry
- **WHEN** `Release(ctx, leaseID)` is called before the lease expires
- **THEN** the port is immediately reclaimed (no need to wait for reaper), and the lease entry is removed

### Requirement: Lease visibility via status endpoint
The system SHALL expose all active leases through `GET /browser/ports/leases`, returning each lease's ID, account ID, port pair, and remaining TTL in seconds.

#### Scenario: Active leases returned
- **WHEN** `GET /browser/ports/leases` is called with 3 active leases
- **THEN** the response is HTTP 200 with `{ "leases": [{ "lease_id", "account_id", "cdp_port", "vnc_port", "expires_in_seconds" }, ...] }`

#### Scenario: No active leases
- **WHEN** `GET /browser/ports/leases` is called with no running containers
- **THEN** the response is `{ "leases": [] }`
