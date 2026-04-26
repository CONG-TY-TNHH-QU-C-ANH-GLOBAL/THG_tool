## Context

The current streaming path is:

```
Browser (frontend canvas)
  ↕ WebSocket  /ws/browser-view/:id
internal/server/cdp_view.go          ← JPEG screencast via CDP
internal/server/vnc_proxy.go         ← noVNC WebSocket proxy to x11vnc
    ↕ TCP 590x
x11vnc (inside Docker container)
    ↕ X11
Xvfb :99 (inside Docker container)
    ↑
Chrome --display=:99
```

Both `cdp_view.go` (CDP JPEG screencast) and `vnc_proxy.go` (noVNC relay) are wired directly into the server's route handlers. Streaming is not a first-class concept — there is no session, no viewer registry, and no way to add a second viewer to the same container. The warm pool and BrowserService have no streaming awareness.

The existing WebSocket route `/ws/browser-view/:id` is used by the frontend canvas. It must continue to function after this change.

This change is **architecture-first** — the goal is to define the interface and migrate existing code behind it. The WebRTC provider is specified but not fully implemented (stub only).

## Goals / Non-Goals

**Goals:**
- `StreamingProvider` interface defined in `internal/streaming/provider.go` with `Attach`, `Detach`, `ListSessions`, `Describe` methods.
- `StreamingSession` model with ID, viewer token, accountID, backend type, connection URL, TTL.
- `VNCStreamingProvider` wrapping existing `cdp_view.go` / `vnc_proxy.go` logic — behavior identical to today, code moved behind the interface.
- `SessionManager` tracking active `StreamingSession` objects per account, enforcing max-viewers cap, cleaning up expired sessions.
- Three new REST endpoints for explicit session lifecycle (attach, detach, info).
- Existing `/ws/browser-view/:id` WebSocket route delegated through `VNCStreamingProvider` — zero behavior change for the frontend.
- `WebRTCStreamingStub` struct implementing the interface with `ErrNotImplemented` returns — placeholder for future work; documents signaling hooks.
- `BrowserService` receives `StreamingProvider` at construction time; calls `provider.Attach(containerInfo)` after a container starts and `provider.Detach(accountID)` before stop.

**Non-Goals:**
- Implementing WebRTC media transport (`pion/webrtc`, STUN/TURN, ICE negotiation) — deferred.
- Multi-node streaming relay (SFU, media server) — deferred.
- Recording or playback of sessions.
- Replacing the CDP screencast path used for internal automation (not viewer-facing) — that path stays in `cdp_view.go` unchanged.
- Removing the VNC port from the `PortRegistry` — VNC ports continue to be allocated per container for the VNC provider.

## Decisions

### 1. `StreamingProvider` interface is session-oriented, not connection-oriented

**Decision**:
```go
type StreamingProvider interface {
    // Called after container start — provider sets up its transport
    Attach(ctx context.Context, info ContainerAttachInfo) (*StreamingEndpoint, error)
    // Called before container stop — provider tears down transport
    Detach(ctx context.Context, accountID int64) error
    // Create a viewer session for this account's stream
    NewSession(ctx context.Context, accountID int64, viewerID string) (*StreamingSession, error)
    // Remove a viewer session
    CloseSession(ctx context.Context, sessionID string) error
    // List active sessions for an account
    ListSessions(ctx context.Context, accountID int64) ([]*StreamingSession, error)
    // Describe the streaming endpoint (URL, protocol, token) for a session
    Describe(ctx context.Context, sessionID string) (*SessionDescriptor, error)
}
```

`ContainerAttachInfo` carries `{AccountID, ContainerID, VNCPort, CDPPort, DisplayNum}`. `StreamingEndpoint` carries `{Protocol: "vnc"|"webrtc", ConnectURL, InternalPort}`.

**Why**: Session-oriented design separates the container-level transport setup (`Attach`/`Detach`) from the per-viewer session (`NewSession`/`CloseSession`). This maps cleanly onto both VNC (one x11vnc process per container, many noVNC WebSocket connections) and WebRTC (one peer connection per viewer, shared media track from one container).

**Alternative considered**: Connection-oriented interface where `Connect(accountID) → ws.Conn` — simpler for VNC but cannot express WebRTC's async signaling (offer/answer exchange is not a single blocking call).

### 2. `VNCStreamingProvider` wraps existing code with no behavioral change

**Decision**: `VNCStreamingProvider` holds the existing `cdpHubs map[int64]*cdpViewHub` and VNC proxy state. `Attach(info)` calls the existing `startAccountScreencast(id, cdpPort)` and records the VNC port. `NewSession(accountID, viewerID)` creates a `StreamingSession` with a signed JWT viewer token and returns the noVNC WebSocket URL (`/ws/browser-view/:id?token=…`). The WebSocket handler validates the token before proxying.

**Why**: Zero behavioral change for current users. The migration is purely structural — existing code moves into the `VNCStreamingProvider` struct, the HTTP route calls `provider.NewSession()` instead of wiring directly, but the WebSocket frame flow is identical.

**Viewer token on existing route**: The existing `/ws/browser-view/:id` route currently requires only JWT auth (staff login). After this change it additionally requires a viewer token from `NewSession`. This is a minor hardening — the web app calls `POST /browser/:id/stream/attach` first, gets a `session_id` + `connect_url`, then opens the WebSocket to `connect_url`. The change is transparent to the user.

### 3. `SessionManager` is a separate struct, not embedded in the provider

**Decision**: `SessionManager` holds `sessions sync.Map[sessionID → *StreamingSession]` and `accountSessions sync.Map[accountID → []sessionID]`. It enforces `STREAM_MAX_VIEWERS_PER_SESSION` and TTL expiry. Each `StreamingProvider` implementation receives a `*SessionManager` reference.

**Why**: Session lifecycle (expiry, max-viewers, token validation) is the same regardless of streaming backend. Putting it in a shared `SessionManager` avoids re-implementing it in every provider and makes the `WebRTCStreamingStub` easier — it can just call `sessionManager.New(...)`.

**Alternative considered**: Embedding session management in each provider — leads to duplication and inconsistent TTL behavior across backends.

### 4. `WebRTCStreamingStub` documents the signaling contract as code comments

**Decision**: `WebRTCStreamingStub` implements `StreamingProvider` with all methods returning `streaming.ErrNotImplemented`. The struct's field comments and method doc-comments explicitly describe: which STUN/TURN servers are needed, what the `Attach` hook must do (start a media pipeline from the display), what `NewSession` must do (create a PeerConnection offer, return SDP in `SessionDescriptor.SDPOffer`), and what the `Describe` response looks like for WebRTC (`{Protocol: "webrtc", SDPOffer: "...", ICEServers: [...]}`).

**Why**: A stub with rich comments is the minimal artifact that enables a future engineer to implement WebRTC without re-reading this design doc. It also forces the interface to be expressive enough for WebRTC before any media code is written.

**Alternative considered**: Just write the interface and a TODO comment — less guidance; the interface might need to change when WebRTC is actually implemented.

### 5. Viewer tokens are short-lived JWTs signed with `JWT_SECRET`

**Decision**: `NewSession` mints a JWT with claims `{session_id, account_id, viewer_id, exp: now + STREAM_SESSION_TTL}`, signed with the existing `JWT_SECRET`. The WebSocket handler calls `sessionManager.ValidateToken(token)` before upgrading. No new secret or key management.

**Why**: Reuses the existing JWT infrastructure. Viewer tokens are short-lived (default 2h) and scoped to one session, so even if leaked they cannot be used to access other accounts.

**Alternative considered**: Random opaque token stored in `SessionManager` — simpler but requires in-process lookup on every WebSocket message. JWT validates statelessly.

### 6. `STREAM_BACKEND` selects provider at startup; no runtime switching

**Decision**: `cmd/scraper/main.go` reads `STREAM_BACKEND` and instantiates the appropriate provider once. The provider reference is passed to `server.New()` and `BrowserService`. Switching backends requires a service restart.

**Why**: Runtime switching would require migrating all active sessions between providers — a complex distributed transaction. For a deployment that starts with VNC and eventually migrates to WebRTC, a rolling restart is the correct migration strategy.

## Risks / Trade-offs

- **Viewer token on existing WebSocket breaks current browser frontend** → Mitigation: the web app `initAccountScreencast()` is updated in the same PR to call `POST /browser/:id/stream/attach` first; the old token-free path is supported for a transition period via a feature flag `STREAM_REQUIRE_SESSION_TOKEN=false` (default `false` until frontend is updated, then `true`).
- **`VNCStreamingProvider.Detach` must tear down all viewer sessions** → Mitigation: `Detach` calls `sessionManager.CloseAll(accountID)` which closes all WebSocket connections for that account; clients receive a close frame and show a "session ended" message.
- **WebRTC stub creates false confidence** → Mitigation: `ErrNotImplemented` is returned at the API layer with HTTP 501; the frontend shows "WebRTC not available" clearly.
- **`SessionManager` `sync.Map` grows if sessions are not closed** → Mitigation: background reaper ticks every `STREAM_SESSION_TTL / 2` and closes expired sessions; max entries bounded by `MAX_CONCURRENT_BROWSERS * STREAM_MAX_VIEWERS_PER_SESSION`.
- **VNCStreamingProvider adds one JWT validation per WebSocket upgrade** → Mitigation: JWT validation is a local HMAC operation (~1µs); not a meaningful latency addition to a WebSocket upgrade that takes ~10ms.

## Migration Plan

1. Create `internal/streaming/` package with `provider.go`, `session.go`, `session_manager.go`, `vnc_provider.go`, `webrtc_stub.go`.
2. Move logic from `cdp_view.go` and `vnc_proxy.go` into `VNCStreamingProvider` methods; leave route registration in `api.go` pointing to provider-delegated handlers.
3. Add `StreamingProvider` field to `Server` struct in `api.go`; update `server.New()` signature.
4. Add `Attach`/`Detach` calls in `DockerBrowserService.Start()` / `Stop()`.
5. Add new REST routes; update frontend.
6. Set `STREAM_REQUIRE_SESSION_TOKEN=false` during initial deployment to preserve existing behavior.
7. Once frontend is verified, set `STREAM_REQUIRE_SESSION_TOKEN=true`.
8. Rollback: revert provider injection — existing `cdp_view.go` and `vnc_proxy.go` code is not deleted until WebRTC is fully implemented.

## Open Questions

- Should `SessionDescriptor.ConnectURL` be an absolute URL (including hostname) or a path? Absolute enables future multi-node routing but requires knowing the public hostname. Path is simpler for single-node. Proposed: path for now, with a `STREAM_PUBLIC_BASE_URL` env var override for future use.
- Should `STREAM_MAX_VIEWERS_PER_SESSION` be enforced per-account or per-container? Currently the same (one container per account), but will diverge if multiple-container-per-account is added. Proposed: per-account for now.
- Who is responsible for closing the WebSocket when `Detach` is called — the provider or the session manager? Proposed: `SessionManager.CloseAll` closes sessions (sends WebSocket close frame); provider calls `CloseAll` from `Detach`.
