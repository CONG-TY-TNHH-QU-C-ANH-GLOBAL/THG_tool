## ADDED Requirements

### Requirement: Start browser container for an account
The system SHALL start a Docker container running Chrome with VNC and CDP access for a given account ID. The container SHALL use a persistent profile directory mounted from the host at `data/profiles/account_{id}/`. The system SHALL reject the request if the per-node concurrency cap has been reached.

#### Scenario: Successful container start
- **WHEN** `POST /browser/start` is called with a valid `account_id`
- **THEN** a Docker container is created and started for that account, dynamic CDP and VNC ports are assigned, and the response includes `{ "account_id", "cdp_port", "vnc_port", "container_id", "status": "running" }`

#### Scenario: Start when concurrency cap is reached
- **WHEN** `POST /browser/start` is called and the number of running containers equals `MAX_CONCURRENT_BROWSERS`
- **THEN** the system returns HTTP 429 with `{ "error": "concurrency limit reached", "limit": <MAX_CONCURRENT_BROWSERS> }`

#### Scenario: Start when container already running
- **WHEN** `POST /browser/start` is called for an account that already has a running container
- **THEN** the system returns the existing container's info with `status: "already_running"` and does not start a duplicate

#### Scenario: Profile directory created if absent
- **WHEN** `POST /browser/start` is called for an account whose profile directory does not exist
- **THEN** the system creates `data/profiles/account_{id}/` before starting the container

### Requirement: Stop browser container for an account
The system SHALL stop and remove the Docker container for a given account ID, and release its allocated ports back to the registry.

#### Scenario: Successful container stop
- **WHEN** `POST /browser/stop` is called with a valid `account_id` that has a running container
- **THEN** the container is stopped and removed, ports are released, and the response is `{ "account_id", "status": "stopped" }`

#### Scenario: Stop when no container is running
- **WHEN** `POST /browser/stop` is called for an account with no running container
- **THEN** the system returns HTTP 404 with `{ "error": "no running container for account" }`

### Requirement: Query browser container status
The system SHALL return the current status of the Docker container for a given account ID, including port assignments and container health.

#### Scenario: Status of running container
- **WHEN** `GET /browser/:id/status` is called for an account with a running container
- **THEN** the response includes `{ "account_id", "status": "running", "cdp_port", "vnc_port", "container_id", "uptime_seconds" }`

#### Scenario: Status of stopped account
- **WHEN** `GET /browser/:id/status` is called for an account with no running container
- **THEN** the response is `{ "account_id", "status": "stopped" }`

### Requirement: Orphan container reconciliation on startup
The system SHALL detect and remove Docker containers from a previous service run that were not cleanly stopped, and release any ports they held, before accepting new start requests.

#### Scenario: Orphan containers found at startup
- **WHEN** the service starts and Docker containers with the label `thg.account_id` exist but are not tracked in the in-memory registry
- **THEN** each orphan container is stopped and removed, and its ports are not re-allocated

#### Scenario: No orphan containers at startup
- **WHEN** the service starts and no labeled Docker containers exist
- **THEN** startup proceeds normally with an empty registry

### Requirement: Container image configurability
The system SHALL use the Docker image specified by the `BROWSER_IMAGE` environment variable, defaulting to `mcr.microsoft.com/playwright:v1.44.0-jammy` if unset.

#### Scenario: Default image used when env var absent
- **WHEN** `BROWSER_IMAGE` is not set in the environment
- **THEN** the service uses `mcr.microsoft.com/playwright:v1.44.0-jammy` as the container image

#### Scenario: Custom image used when env var set
- **WHEN** `BROWSER_IMAGE=custom-registry/browser:latest` is set
- **THEN** the service uses `custom-registry/browser:latest` for all new containers
