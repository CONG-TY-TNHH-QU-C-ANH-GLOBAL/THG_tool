## Context

The system currently uses `internal/workspace/workspace.go` to spawn Chrome processes directly on the host OS. Each account gets a process with a dedicated `data/profiles/account_{id}/` directory, but all processes share the same host network namespace, display server, and process table. This works for a handful of accounts but breaks down at scale:

- Port range for CDP/VNC is manually carved out and conflicts arise.
- A crashing Chrome process can leave zombie children or lock profile directories.
- No resource isolation: one runaway Chrome consumes all CPU/RAM on the node.
- Linux-only (relies on Xvfb/x11vnc display manager in `internal/browser/display.go`).

The host runs Docker already (CI/CD pipeline uses it). The Go server binary can reach the Docker daemon via `/var/run/docker.sock`.

## Goals / Non-Goals

**Goals:**
- Each account maps to exactly one Docker container running a Playwright-Chromium image.
- Containers are ephemeral (stop = remove), but profiles are persistent (host-mounted volume).
- CDP port and VNC port are dynamically allocated from configured ranges; registry prevents conflicts.
- A configurable concurrency cap (`MAX_CONCURRENT_BROWSERS`) is enforced before starting a new container.
- Three REST endpoints expose the service: `POST /browser/start`, `POST /browser/stop`, `GET /browser/:id/status`.
- Existing WebSocket screencast (`/ws/browser-view/:id`) continues to work by connecting to the container's CDP port.

**Non-Goals:**
- Multi-node scheduling (Kubernetes, Swarm) — single-node Docker only for now.
- Container image building — use an official Playwright image.
- Browser session sharing between accounts.
- Windows host support (Docker socket path is Linux-specific; PowerShell Docker SDK is out of scope).

## Decisions

### 1. Use Go Docker SDK instead of shelling out to `docker` CLI

**Decision**: Import `github.com/docker/docker/client` and call the API directly.

**Why**: Shell-out is fragile (PATH dependency, output parsing, no typed errors). The SDK gives typed responses, context cancellation, and is idiomatic Go.

**Alternative considered**: `docker` CLI via `exec.Command` — rejected because stdout parsing for dynamic port info is error-prone.

### 2. Port allocation via an in-process registry with mutex

**Decision**: `PortRegistry` struct holds two monotonically-advancing counters (one for CDP, one for VNC) within configured ranges. `Acquire()` scans the range for a free port (binds a listener briefly to test), `Release()` marks it free.

**Why**: Avoids a separate service or persistent store. Port ranges are small (e.g., 100 slots each). The brief bind-test guarantees the port is actually free on the host before Docker maps it.

**Alternative considered**: Pre-assigning ports at account creation time — rejected because it couples account management to infrastructure config.

### 3. Playwright-Chromium Docker image as the browser base

**Decision**: Default image `mcr.microsoft.com/playwright:v1.44.0-jammy` (or configurable via `BROWSER_IMAGE` env var). Entrypoint overridden to launch `google-chrome` (bundled in the Playwright image) with CDP and VNC flags.

**Why**: Playwright images include all system deps (fonts, codecs, display libs) needed for headless Chrome + VNC. Avoids maintaining a custom Dockerfile.

**Alternative considered**: Custom `Dockerfile` with `google-chrome-stable` — more control but adds image build step to CI and ops burden.

### 4. VNC via x11vnc inside the container

**Decision**: Container entrypoint script:
1. Start `Xvfb :99`
2. Start `x11vnc -display :99 -rfbport <VNC_PORT> -forever -nopw`
3. Start Chrome with `--display=:99 --remote-debugging-port=<CDP_PORT>`

Host maps `VNC_PORT` and `CDP_PORT` from the dynamic registry.

**Why**: Mirrors the existing `internal/browser/display.go` approach but isolated per container. No change needed to the frontend VNC viewer or CDP screencast code.

### 5. `BrowserService` wraps Docker SDK; `workspace.Manager` kept for local-dev fallback

**Decision**: New `internal/browser/service.go` implements a `BrowserServicer` interface. The existing `workspace.Manager` also satisfies this interface. `server.New()` accepts the interface, so local dev can still use the direct-process manager by setting `BROWSER_BACKEND=local`.

**Why**: Allows gradual migration; dev machines without Docker can still run the system. No flag day.

## Risks / Trade-offs

- **Docker socket access grants root-equivalent power** → Mitigation: document that the service binary should run as a dedicated non-root user with group `docker`; never expose the socket endpoint publicly.
- **Port range exhaustion if containers are not cleaned up** → Mitigation: `BrowserService.Stop()` always calls `PortRegistry.Release()`; a startup reconciliation loop removes orphaned containers from prior crashes.
- **Container startup latency (~2–4s)** → Mitigation: pre-pull the image on service startup; show a loading state in the UI during start. Acceptable for manual "start account" flows.
- **`MAX_CONCURRENT_BROWSERS` cap causes "429 Too Many" errors under load** → Mitigation: expose the limit in `GET /browser/:id/status` so the UI can surface capacity clearly.
- **Profile mount permissions** → Mitigation: container runs as UID 1000; host profile dirs created with same UID. Document in deployment guide.

## Migration Plan

1. Add `github.com/docker/docker` to `go.mod` (`go get github.com/docker/docker/client`).
2. Implement `internal/browser/port_registry.go` and `internal/browser/service.go`.
3. Update `internal/server/workspace_handlers.go` to delegate to `BrowserServicer` interface.
4. Update `cmd/scraper/main.go`: choose `BrowserService` (Docker) or `workspace.Manager` (local) based on `BROWSER_BACKEND` env var.
5. Add new env vars to `.env.example`.
6. Deploy: pull Playwright image on server before restarting service (`docker pull mcr.microsoft.com/playwright:v1.44.0-jammy`).
7. Rollback: set `BROWSER_BACKEND=local`, restart — reverts to prior process-based manager with no data loss.

## Open Questions

- Should `GET /browser/:id/status` also return container CPU/memory stats (via Docker stats API)? Useful for ops but adds polling overhead.
- Long term: should port allocation persist to SQLite so ports survive service restarts without orphaned containers? Currently registry is in-memory and reconciled at startup.
