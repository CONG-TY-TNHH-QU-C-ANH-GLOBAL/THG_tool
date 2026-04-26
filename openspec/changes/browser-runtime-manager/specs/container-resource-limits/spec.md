## ADDED Requirements

### Requirement: CPU quota enforcement at container creation
The system SHALL enforce a CPU quota on each browser container by setting `HostConfig.NanoCPUs = CONTAINER_CPU_QUOTA * 1e9` when creating the Docker container. `CONTAINER_CPU_QUOTA` is a float representing fractional CPU cores (e.g., `0.5` = half a core). A value of `0` means no limit.

#### Scenario: CPU quota applied
- **WHEN** `CONTAINER_CPU_QUOTA=1.5` is set and a container is created
- **THEN** `HostConfig.NanoCPUs` is set to `1500000000` (1.5 * 1e9) in the Docker create call

#### Scenario: No CPU limit when zero
- **WHEN** `CONTAINER_CPU_QUOTA=0` (default) and a container is created
- **THEN** `HostConfig.NanoCPUs` is not set (or set to 0); Docker applies no CPU quota

### Requirement: Memory limit enforcement at container creation
The system SHALL enforce a memory limit on each browser container by setting `HostConfig.Memory = CONTAINER_MEMORY_LIMIT` (in bytes) when creating the Docker container. `CONTAINER_MEMORY_LIMIT` is specified in bytes in the environment. A value of `0` means no limit.

#### Scenario: Memory limit applied
- **WHEN** `CONTAINER_MEMORY_LIMIT=1073741824` (1GB) is set and a container is created
- **THEN** `HostConfig.Memory` is set to `1073741824` in the Docker create call

#### Scenario: No memory limit when zero
- **WHEN** `CONTAINER_MEMORY_LIMIT=0` (default) and a container is created
- **THEN** `HostConfig.Memory` is not set (or set to 0); Docker applies no memory limit

### Requirement: Resource limits stored in `browser_containers`
The system SHALL store the effective `cpu_quota` and `memory_limit` values in the `browser_containers` row at container creation time so that `GET /browser/:id/runtime` can report them without a Docker inspect API call.

#### Scenario: Limits persisted on create
- **WHEN** a container is successfully created with non-zero limits
- **THEN** `browser_containers.cpu_quota` is set to `CONTAINER_CPU_QUOTA * 1e9` (the raw NanoCPU value) and `browser_containers.memory_limit` is set to `CONTAINER_MEMORY_LIMIT` (bytes)

#### Scenario: Zero limits stored as zero
- **WHEN** a container is created with default limits (both zero)
- **THEN** `browser_containers.cpu_quota=0` and `browser_containers.memory_limit=0`; the runtime endpoint reports `0` for both fields, meaning no limit

### Requirement: Resource limit configuration
The system SHALL read `CONTAINER_CPU_QUOTA` (default `0`, float) and `CONTAINER_MEMORY_LIMIT` (default `0`, integer bytes) from `internal/config/config.go` and apply them to all containers created by `DockerBrowserService`.

#### Scenario: Default values mean no limits
- **WHEN** neither `CONTAINER_CPU_QUOTA` nor `CONTAINER_MEMORY_LIMIT` is set in the environment
- **THEN** containers are created with no CPU or memory constraints; existing behavior is unchanged
