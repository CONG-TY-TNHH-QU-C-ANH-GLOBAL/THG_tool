## ADDED Requirements

### Requirement: VNC provider wraps existing screencast and proxy logic
The system's `VNCStreamingProvider` SHALL encapsulate all logic currently in `internal/server/cdp_view.go` (CDP JPEG screencast hub) and `internal/server/vnc_proxy.go` (noVNC WebSocket proxy). Existing streaming behavior SHALL be preserved exactly — JPEG frame delivery cadence, input forwarding, and reconnection handling are unchanged.

#### Scenario: CDP screencast starts on Attach
- **WHEN** `VNCStreamingProvider.Attach(ctx, info)` is called for account 42
- **THEN** the CDP screencast hub for account 42 is started (equivalent to existing `startAccountScreencast(42, cdpPort)`); the returned `StreamingEndpoint` has `Protocol: "vnc"` and `ConnectURL: "/ws/browser-view/42"`

#### Scenario: CDP screencast stops on Detach
- **WHEN** `VNCStreamingProvider.Detach(ctx, 42)` is called
- **THEN** the CDP screencast hub for account 42 is stopped (equivalent to existing `stopAccountScreencast(42)`); all viewer WebSockets are closed

### Requirement: VNC NewSession returns noVNC WebSocket URL
The system's `VNCStreamingProvider.NewSession` SHALL create a `StreamingSession` via `SessionManager`, set `ConnectURL` to `/ws/browser-view/:id?session_token=<token>`, and return it. The existing `/ws/browser-view/:id` WebSocket handler SHALL use this path to serve JPEG frames.

#### Scenario: NewSession produces valid connect URL
- **WHEN** `VNCStreamingProvider.NewSession(ctx, 42, "viewer-001")` is called
- **THEN** the returned session has `Protocol: "vnc"`, `ConnectURL: "/ws/browser-view/42?session_token=<jwt>"`, and a non-zero `ExpiresAt`

#### Scenario: Multiple concurrent viewer sessions
- **WHEN** two clients call `POST /browser/42/stream/attach` simultaneously (within `STREAM_MAX_VIEWERS_PER_SESSION` limit)
- **THEN** each receives a distinct `session_id` and `viewer_token`; both WebSocket connections receive the same JPEG frames from the shared CDP screencast hub

### Requirement: VNC provider is default when STREAM_BACKEND unset
The system SHALL use `VNCStreamingProvider` when `STREAM_BACKEND` is not set or is set to `vnc`, with no change in behavior from the pre-abstraction implementation.

#### Scenario: Backward-compatible default
- **WHEN** `STREAM_BACKEND` is not configured and a container starts
- **THEN** `VNCStreamingProvider.Attach` is called; `GET /browser/:id/stream/info` returns `"protocol": "vnc"` and a `connect_url` beginning with `/ws/browser-view/`

#### Scenario: Existing WebSocket route still works
- **WHEN** the frontend opens a WebSocket to `/ws/browser-view/42` (with or without session token depending on `STREAM_REQUIRE_SESSION_TOKEN`)
- **THEN** the connection is accepted and JPEG frames are delivered identically to pre-abstraction behavior
