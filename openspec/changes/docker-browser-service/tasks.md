## 1. Dependencies and Config

- [ ] 1.1 Add `github.com/docker/docker/client` to `go.mod` and `go.sum` via `go get`
- [ ] 1.2 Add new env vars to `internal/config/config.go`: `BrowserBackend`, `BrowserImage`, `BrowserCDPPortRange`, `BrowserVNCPortRange`, `MaxConcurrentBrowsers`
- [ ] 1.3 Add corresponding entries to `.env.example` with documented defaults

## 2. Port Registry

- [ ] 2.1 Create `internal/browser/port_registry.go` — `PortRegistry` struct with mutex-guarded `allocated map[int]bool`
- [ ] 2.2 Implement `NewPortRegistry(cdpRange, vncRange string, maxConcurrent int) (*PortRegistry, error)` — parse and validate range strings, fail on bad format
- [ ] 2.3 Implement `Acquire() (cdpPort, vncPort int, err error)` — scan ranges, brief TCP bind test per candidate, return error when cap reached or range exhausted
- [ ] 2.4 Implement `Release(cdpPort, vncPort int)` — mark ports free in the map
- [ ] 2.5 Write unit tests for `PortRegistry`: valid acquire, cap enforcement, range exhaustion, invalid range format

## 3. Browser Container Service

- [ ] 3.1 Create `internal/browser/service.go` — define `BrowserServicer` interface with `Start`, `Stop`, `Status`, `Reconcile` methods
- [ ] 3.2 Implement `DockerBrowserService` struct with Docker SDK client, `PortRegistry`, and `running map[int64]*ContainerInfo`
- [ ] 3.3 Implement `NewDockerBrowserService(cfg Config, portRegistry *PortRegistry) (*DockerBrowserService, error)` — connect to Docker daemon via socket
- [ ] 3.4 Implement `Start(ctx, accountID int64) (*ContainerInfo, error)` — check cap, create profile dir, acquire ports, pull-if-needed, `docker run` with label `thg.account_id`, host-mount profile, map ports
- [ ] 3.5 Implement `Stop(ctx, accountID int64) error` — `docker stop` + `docker rm`, release ports, remove from `running` map
- [ ] 3.6 Implement `Status(ctx, accountID int64) (*StatusInfo, error)` — check `running` map, query Docker inspect for uptime
- [ ] 3.7 Implement `Reconcile(ctx) error` — list containers with label `thg.account_id`, stop+remove any not in `running` map (orphan cleanup)
- [ ] 3.8 Write integration test for `DockerBrowserService.Start` + `Stop` against a real Docker daemon (skip if Docker unavailable)

## 4. REST Handlers

- [ ] 4.1 Create `internal/server/browser_handlers.go` — `BrowserHandlers` struct wrapping `BrowserServicer`
- [ ] 4.2 Implement `POST /browser/start` handler — parse `account_id`, call `BrowserServicer.Start`, return JSON response; HTTP 429 on cap exceeded
- [ ] 4.3 Implement `POST /browser/stop` handler — parse `account_id`, call `BrowserServicer.Stop`, return JSON; HTTP 404 if not running
- [ ] 4.4 Implement `GET /browser/:id/status` handler — call `BrowserServicer.Status`, return JSON
- [ ] 4.5 Register new routes in `internal/server/api.go` under `/browser/*`

## 5. Wiring in main.go

- [ ] 5.1 In `cmd/scraper/main.go`, read `BROWSER_BACKEND` env var; if `"docker"`, instantiate `PortRegistry` + `DockerBrowserService`; else fall back to existing `workspace.Manager`
- [ ] 5.2 Call `BrowserService.Reconcile(ctx)` during startup before the HTTP server begins accepting requests
- [ ] 5.3 Ensure `BrowserService` is shut down (and all containers stopped) in the `defer` / shutdown path

## 6. Workspace Handlers Migration

- [ ] 6.1 Update `internal/server/workspace_handlers.go` to delegate `start`/`stop`/`status` to `BrowserServicer` interface instead of calling `workspace.Manager` directly
- [ ] 6.2 Alias old routes (`/api/browser/workspaces/:id/start`, etc.) to new handlers so existing frontend JS continues to work without immediate changes
- [ ] 6.3 Confirm existing WebSocket screencast (`/ws/browser-view/:id`) still connects via the dynamically assigned CDP port from `ContainerInfo`

## 7. Frontend Updates

- [ ] 7.1 Update `internal/server/static/app.js` `browserStartAccount()` and `browserStopAccount()` to call the new `/browser/start` and `/browser/stop` endpoints
- [ ] 7.2 Update status pill rendering to use `GET /browser/:id/status` response shape
- [ ] 7.3 Display concurrency cap error (HTTP 429) as a user-visible toast/message in the Browser page

## 8. Documentation and Config

- [ ] 8.1 Add Docker deployment notes to `CLAUDE.md`: required env vars, image pre-pull command, Docker socket permission setup
- [ ] 8.2 Verify `go build ./cmd/scraper/` succeeds with no new warnings
- [ ] 8.3 Manual smoke test: start the service with `BROWSER_BACKEND=docker`, start a container for one account, verify CDP and VNC ports are reachable, stop the container, verify ports released
