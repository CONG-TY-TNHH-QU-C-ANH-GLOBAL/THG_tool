## ADDED Requirements

### Requirement: Dynamic port acquisition for CDP and VNC
The system SHALL maintain an in-memory registry of available ports within configured ranges and assign a free CDP port and a free VNC port to each new browser container. A port SHALL be considered free only if no other running container holds it AND the host OS confirms it is not in use.

#### Scenario: Ports acquired successfully
- **WHEN** a new container is being started and free ports are available in both ranges
- **THEN** the registry returns a unique `(cdp_port, vnc_port)` pair not held by any running container

#### Scenario: CDP port range exhausted
- **WHEN** all ports in the CDP range are allocated
- **THEN** the registry returns an error and the container start is aborted

#### Scenario: VNC port range exhausted
- **WHEN** all ports in the VNC range are allocated
- **THEN** the registry returns an error and the container start is aborted

#### Scenario: OS-level port conflict detected
- **WHEN** a candidate port is free in the registry but a brief TCP bind test fails (port occupied by another process)
- **THEN** the registry skips that port and tries the next candidate in the range

### Requirement: Port release on container stop
The system SHALL return allocated ports to the registry when a container is stopped, making them available for future containers.

#### Scenario: Ports released after stop
- **WHEN** `POST /browser/stop` completes successfully
- **THEN** the CDP and VNC ports previously assigned to that container are marked free in the registry and may be re-assigned to future containers

#### Scenario: Stop failure does not leak ports
- **WHEN** Docker fails to remove the container but the stop command was issued
- **THEN** ports are still released in the registry (best-effort cleanup)

### Requirement: Configurable port ranges
The system SHALL read port range boundaries from environment variables `BROWSER_CDP_PORT_RANGE` and `BROWSER_VNC_PORT_RANGE`, each in `<start>-<end>` format (e.g., `9222-9322`).

#### Scenario: Valid range configured
- **WHEN** `BROWSER_CDP_PORT_RANGE=9222-9322` is set
- **THEN** the registry allocates CDP ports only within [9222, 9322]

#### Scenario: Invalid range format causes startup failure
- **WHEN** `BROWSER_CDP_PORT_RANGE=abc` or an inverted range is set
- **THEN** the service fails to start with a clear configuration error message

#### Scenario: Default ranges used when env vars absent
- **WHEN** neither `BROWSER_CDP_PORT_RANGE` nor `BROWSER_VNC_PORT_RANGE` is set
- **THEN** the registry uses `9222-9322` for CDP and `5900-6000` for VNC

### Requirement: Concurrency cap enforcement
The system SHALL enforce a maximum number of simultaneously held port pairs, equal to `MAX_CONCURRENT_BROWSERS`. Acquisition attempts beyond this cap SHALL fail immediately.

#### Scenario: Acquisition blocked at cap
- **WHEN** the number of currently allocated port pairs equals `MAX_CONCURRENT_BROWSERS`
- **THEN** `Acquire()` returns an error without allocating a new pair

#### Scenario: Cap increase at runtime is not supported
- **WHEN** `MAX_CONCURRENT_BROWSERS` is changed in the environment after startup
- **THEN** the cap remains at the value read at startup (restart required to change)
