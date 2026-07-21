> **Lifecycle status (2026-07-21 spec IA reconciliation):** proposal only — nothing under `openspec/` is current runtime authority (per `AGENTS.md`/`CLAUDE.md`; the runtime authority is `specs/domains/platform-foundation/features/runtime-topology/technical.md`). NOT IMPLEMENTED. Production realized the visible workspace Chrome + extension connector path instead; hidden per-account Docker containers conflict with the merged account-safety doctrine (operator-observable browsers, no hidden browser pools) and would require founder re-approval.

## Why

The current workspace system launches Chrome directly on the host process, meaning all accounts share the same OS environment and competing for the same ports, file handles, and display resources. As account count grows past a handful, this creates port conflicts, profile corruption, and no isolation guarantees. We need a container-based approach where each Chrome instance runs in its own Docker container with a dedicated profile, VNC port, and CDP port, so the system can scale to hundreds of concurrent browsers reliably.

## What Changes

- Introduce a `BrowserService` that manages Chrome-in-Docker containers (one per account).
- Replace direct `workspace.Manager` Chrome process spawning with Docker container lifecycle management.
- Add dynamic port allocation for VNC (5900-range) and CDP (9222-range) per container.
- Mount persistent profile directories from host into containers (`data/profiles/account_{id}/`).
- Expose three new REST endpoints: `POST /browser/start`, `POST /browser/stop`, `GET /browser/:id/status`.
- Add container concurrency cap (`MAX_CONCURRENT_BROWSERS`) enforced at the service layer.
- Existing `workspace_handlers.go` routes updated to delegate to `BrowserService` instead of local process manager.

## Capabilities

### New Capabilities

- `browser-container-lifecycle`: Start, stop, and query isolated Chrome Docker containers per account, with dynamic port allocation and persistent profile mounts.
- `browser-port-registry`: Track allocated VNC and CDP port pairs across all running containers to prevent conflicts and enforce per-node concurrency limits.

### Modified Capabilities

<!-- No existing specs yet — no delta files needed -->

## Impact

- **Code**: `internal/workspace/workspace.go` replaced by `internal/browser/container.go` + `internal/browser/port_registry.go`; `internal/server/workspace_handlers.go` updated; `cmd/scraper/main.go` wiring updated.
- **APIs**: New endpoints `POST /browser/start`, `POST /browser/stop`, `GET /browser/:id/status` (old `/api/browser/workspaces/*` routes deprecated or aliased).
- **Dependencies**: Requires Docker daemon on the host; Go `docker/docker` client SDK added (`github.com/docker/docker/client`); Playwright-Chromium Docker image (e.g., `mcr.microsoft.com/playwright:v1.44.0-jammy`).
- **Infrastructure**: Host must have Docker installed and the browser service process must have access to the Docker socket (`/var/run/docker.sock`).
- **Config**: New env vars `MAX_CONCURRENT_BROWSERS`, `BROWSER_IMAGE`, `BROWSER_VNC_PORT_RANGE`, `BROWSER_CDP_PORT_RANGE`.
