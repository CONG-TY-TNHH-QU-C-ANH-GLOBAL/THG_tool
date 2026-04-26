## 1. Config and Environment

- [ ] 1.1 Add `STREAM_BACKEND` (default `vnc`), `STREAM_SESSION_TTL` (default `2h`), `STREAM_MAX_VIEWERS_PER_SESSION` (default `5`), `STREAM_REQUIRE_SESSION_TOKEN` (default `false`), `STREAM_PUBLIC_BASE_URL` (default empty) to `internal/config/config.go`
- [ ] 1.2 Add all new env vars to `.env.example` with documented defaults and a note that `STREAM_REQUIRE_SESSION_TOKEN` should be set to `true` after frontend is updated

## 2. Core Interface and Models

- [ ] 2.1 Create `internal/streaming/provider.go` — define `ContainerAttachInfo`, `StreamingEndpoint`, `SessionDescriptor`, `ICEServer` structs; define `StreamingProvider` interface with `Attach`, `Detach`, `NewSession`, `CloseSession`, `ListSessions`, `Describe` methods; define `ErrNotImplemented` sentinel error
- [ ] 2.2 Create `internal/streaming/session.go` — define `StreamingSession` struct (`SessionID`, `AccountID`, `ViewerID`, `Protocol`, `ConnectURL`, `ViewerToken`, `ExpiresAt`, `wsConn *websocket.Conn`); define `SessionManager` struct with `sessions sync.Map` and `accountIndex sync.Map`
- [ ] 2.3 Implement `SessionManager.New(accountID int64, viewerID, protocol, connectURL string, ttl time.Duration) (*StreamingSession, error)` — enforce `STREAM_MAX_VIEWERS_PER_SESSION`, mint JWT viewer token signed with `JWT_SECRET`, store session
- [ ] 2.4 Implement `SessionManager.Close(sessionID string) error` — send WebSocket close frame 1000, delete from both maps
- [ ] 2.5 Implement `SessionManager.CloseAll(accountID int64)` — close all sessions for account, send WebSocket close frame 1001
- [ ] 2.6 Implement `SessionManager.ValidateToken(token string) (*StreamingSession, error)` — parse and verify JWT, return session or error
- [ ] 2.7 Implement `SessionManager.startReaper(ctx context.Context)` — ticker at `STREAM_SESSION_TTL / 2`; close and remove sessions where `time.Now().After(session.ExpiresAt)` with close code 1001
- [ ] 2.8 Write unit tests: New enforces max-viewers, token validates correctly, Close sends close frame, CloseAll closes all sessions for account, reaper removes expired

## 3. VNC Streaming Provider

- [ ] 3.1 Create `internal/streaming/vnc_provider.go` — `VNCStreamingProvider` struct holding `cdpHubs map[int64]*cdpViewHub` (moved from `Server`), `sessionManager *SessionManager`, config
- [ ] 3.2 Implement `VNCStreamingProvider.Attach(ctx, info ContainerAttachInfo) (*StreamingEndpoint, error)` — call existing `startAccountScreencast(info.AccountID, info.CDPPort)` logic; return `StreamingEndpoint{Protocol: "vnc", ConnectURL: "/ws/browser-view/<id>"}`
- [ ] 3.3 Implement `VNCStreamingProvider.Detach(ctx, accountID int64) error` — call `stopAccountScreencast(accountID)`; call `sessionManager.CloseAll(accountID)`
- [ ] 3.4 Implement `VNCStreamingProvider.NewSession(ctx, accountID int64, viewerID string) (*StreamingSession, error)` — call `sessionManager.New(...)` with `ConnectURL = "/ws/browser-view/<id>?session_token=<token>"`
- [ ] 3.5 Implement `VNCStreamingProvider.CloseSession`, `ListSessions`, `Describe` delegating to `sessionManager`
- [ ] 3.6 Move `cdpViewHub` and `startAccountScreencast`/`stopAccountScreencast` logic from `internal/server/cdp_view.go` into `VNCStreamingProvider`; leave `cdp_view.go` as a thin file that registers the WebSocket route
- [ ] 3.7 Write unit tests: Attach starts hub, Detach stops hub and closes sessions, NewSession returns valid connect URL, multiple sessions up to max

## 4. WebRTC Stub

- [ ] 4.1 Create `internal/streaming/webrtc_stub.go` — `WebRTCStreamingStub` struct with doc-comments on each field describing the STUN/TURN config needed and the GStreamer pipeline hook
- [ ] 4.2 Implement all `StreamingProvider` methods returning `streaming.ErrNotImplemented` with descriptive error messages
- [ ] 4.3 Add method-level doc-comments to `Attach` (describe display → media pipeline requirement), `NewSession` (describe SDP offer/answer flow and ICE candidate exchange), `Describe` (describe `SDPOffer` and `ICEServers` fields in `SessionDescriptor`)
- [ ] 4.4 Write a compile-time interface check: `var _ streaming.StreamingProvider = (*WebRTCStreamingStub)(nil)`

## 5. REST Handlers

- [ ] 5.1 Create `internal/server/streaming_handlers.go` — `StreamingHandlers` struct holding `StreamingProvider` reference and config
- [ ] 5.2 Implement `POST /browser/:id/stream/attach` handler — validate container is running, call `provider.NewSession(accountID, viewerID)`, return HTTP 201 with session JSON; return 404 or 429 on error
- [ ] 5.3 Implement `DELETE /browser/:id/stream/detach` handler — parse `session_id` query param, call `provider.CloseSession(sessionID)`, return HTTP 200 or 404
- [ ] 5.4 Implement `GET /browser/:id/stream/info` handler — call `provider.ListSessions(accountID)`, return session list JSON
- [ ] 5.5 Update `/ws/browser-view/:id` WebSocket handler in `cdp_view.go`: when `STREAM_REQUIRE_SESSION_TOKEN=true`, parse `session_token` query param, call `sessionManager.ValidateToken`, reject with HTTP 401 if invalid; when false, skip token check
- [ ] 5.6 Register all three new routes in `internal/server/api.go`; pass `StreamingProvider` to `StreamingHandlers`

## 6. BrowserService Integration

- [ ] 6.1 Add `StreamingProvider streaming.StreamingProvider` field to `DockerBrowserService` struct
- [ ] 6.2 Update `NewDockerBrowserService` constructor to accept `streaming.StreamingProvider` parameter
- [ ] 6.3 In `DockerBrowserService.Start()`, after container is running, call `provider.Attach(ctx, ContainerAttachInfo{...})`; on `Attach` error, stop and remove container and return error
- [ ] 6.4 In `DockerBrowserService.Stop()`, call `provider.Detach(ctx, accountID)` before `docker stop`; log WARN on error but continue with stop
- [ ] 6.5 Store `StreamingEndpoint` returned by `Attach` in `ContainerInfo.StreamingEndpoint`

## 7. Server and main.go Wiring

- [ ] 7.1 Remove `cdpHubs map[int64]*cdpViewHub` from `Server` struct in `api.go` — now owned by `VNCStreamingProvider`
- [ ] 7.2 Add `StreamingProvider streaming.StreamingProvider` field to `Server` struct; update `server.New()` to accept it
- [ ] 7.3 In `cmd/scraper/main.go`, read `STREAM_BACKEND`; instantiate `VNCStreamingProvider` or `WebRTCStreamingStub`; pass `SessionManager` (with reaper started) to the provider
- [ ] 7.4 Pass `StreamingProvider` to `DockerBrowserService` constructor and to `server.New()`
- [ ] 7.5 Start `SessionManager` reaper goroutine in `main.go` after provider is instantiated

## 8. Frontend Updates

- [ ] 8.1 Update `initAccountScreencast(accountID)` in `app.js`: call `POST /browser/${accountID}/stream/attach` first; extract `connect_url` and `viewer_token` from response; open WebSocket to `connect_url` (which includes the token)
- [ ] 8.2 Update `stopAccountScreencast(accountID)` to call `DELETE /browser/${accountID}/stream/detach?session_id=<id>` when closing viewer
- [ ] 8.3 Handle WebSocket close frames 1000 (detached) and 1001 (expired/container stopped) — show appropriate message in the browser canvas area

## 9. Verification

- [ ] 9.1 `go build ./cmd/scraper/` passes; compile-time interface checks for both `VNCStreamingProvider` and `WebRTCStreamingStub` pass
- [ ] 9.2 `go test ./internal/streaming/...` — all unit tests pass with `-race`
- [ ] 9.3 Manual test with `STREAM_BACKEND=vnc` and `STREAM_REQUIRE_SESSION_TOKEN=false`: existing browser canvas works identically to pre-abstraction
- [ ] 9.4 Manual test with `STREAM_REQUIRE_SESSION_TOKEN=true`: `POST /browser/42/stream/attach` returns session; WebSocket with token opens successfully; WebSocket without token returns 401
- [ ] 9.5 Manual test: start container, open two viewer sessions (two browser tabs); confirm both receive JPEG frames; stop container; confirm both WebSockets receive close frame 1001
- [ ] 9.6 Manual test with `STREAM_BACKEND=webrtc`: `POST /browser/42/stream/attach` returns HTTP 501 with descriptive error
