> **Lifecycle status (2026-07-21 spec IA reconciliation):** proposal only — nothing under `openspec/` is current runtime authority (per `AGENTS.md`/`CLAUDE.md`; the runtime authority is `specs/domains/platform-foundation/features/runtime-topology/technical.md`). NOT IMPLEMENTED (depends on the unimplemented docker-browser-service). Visible workspace session lifecycle is handled by `internal/workspace` + `internal/session` instead.

## Why

The `docker-browser-service` change creates and removes Docker containers on demand, but treats each container as a fire-and-forget object with no lifecycle state machine, no health probing, and no automatic recovery when a container enters a partial or failed state (created but not started, running but unresponsive, OOM-killed). A `BrowserRuntimeManager` that owns the complete single-node container lifecycle — from creation through health-verified running through clean shutdown — makes the system self-healing and operationally predictable.

## What Changes

- Introduce a `BrowserRuntimeManager` that wraps `DockerBrowserService` and adds a formal container lifecycle state machine: `pending → creating → starting → running → unhealthy → stopping → stopped → removed`.
- Add a `ContainerHealthProbe` that periodically checks each running container via its CDP `/json/version` endpoint; transitions unhealthy containers to the `unhealthy` state and triggers restart per the configured restart policy.
- Add a configurable `RestartPolicy` per container (or globally): `never`, `on-failure` (up to N times), `always`. Crashed containers matching the policy are automatically re-queued.
- Add CPU and memory resource limits enforced at container creation time via Docker `HostConfig.Resources`.
- Persist container lifecycle state to SQLite (`browser_containers` table) so the manager can reconcile actual Docker state against expected state on startup.
- Expose `GET /browser/:id/runtime` returning the full lifecycle state, restart count, last health check result, and resource usage.

## Capabilities

### New Capabilities

- `container-lifecycle-fsm`: Formal state machine for browser container lifecycle with valid transitions, persistence in SQLite, and event log.
- `container-health-probe`: CDP-endpoint health check per running container on a configurable interval; drives `running → unhealthy` transitions.
- `container-restart-policy`: Per-container or global restart policy (`never`, `on-failure:<N>`, `always`); auto-requeue on qualifying crash or health failure.
- `container-resource-limits`: CPU quota and memory limit enforced at Docker container creation via `HostConfig.NanoCPUs` and `HostConfig.Memory`.

### Modified Capabilities

- `browser-container-lifecycle`: `POST /browser/start` and `POST /browser/stop` now delegate to `BrowserRuntimeManager` instead of directly to `DockerBrowserService`. The visible API behavior is unchanged but lifecycle state transitions are tracked and containers are auto-recovered. Requires delta spec.

## Impact

- **Code**: New `internal/browser/runtime_manager.go`, `internal/browser/health_probe.go`, `internal/browser/restart_policy.go`; new `browser_containers` table migration in `store.go`; `DockerBrowserService` becomes an internal implementation detail called only by `BrowserRuntimeManager`; new `GET /browser/:id/runtime` handler.
- **APIs**: New read-only `GET /browser/:id/runtime` endpoint. All existing `/browser/start` and `/browser/stop` behavior preserved.
- **Config**: `CONTAINER_HEALTH_PROBE_INTERVAL` (default `10s`), `CONTAINER_HEALTH_TIMEOUT` (default `3s`), `CONTAINER_RESTART_POLICY` (default `on-failure:3`), `CONTAINER_CPU_QUOTA` (default `0` = no limit), `CONTAINER_MEMORY_LIMIT` (default `0` = no limit).
- **Database**: New `browser_containers` table (`account_id`, `container_id`, `state`, `restart_count`, `last_health_at`, `last_health_ok`, `created_at`, `updated_at`).
- **No new external dependencies** — uses existing Docker SDK and SQLite.
