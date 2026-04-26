## Why

VNC is hard-coded into each Docker container (one x11vnc process per container, one noVNC WebSocket proxy per viewer), meaning every streaming session opens a persistent TCP tunnel through the server regardless of viewer count, and the VNC protocol was not designed for low-latency high-concurrency web delivery. Abstracting the streaming layer now — while the container model is still young — lets future streaming backends (WebRTC, CDP screencast relay) be swapped in without touching `BrowserService`, the warm pool, or the scheduler.

## What Changes

- Introduce a `StreamingProvider` interface that `BrowserService` uses to attach, detach, and route viewers to a container's display, replacing all direct VNC/noVNC wiring.
- Refactor the existing VNC implementation into a `VNCStreamingProvider` that satisfies the interface and is the default backend.
- Add a `StreamingSession` concept: a per-viewer token-gated session with an independent lifecycle from the container itself.
- Expose viewer management APIs: `POST /browser/:id/stream/attach`, `DELETE /browser/:id/stream/detach`, `GET /browser/:id/stream/info` — replacing the implicit "open WebSocket → you get VNC" model.
- Define the `WebRTCStreamingProvider` interface contract (spec + stub) so a future implementation has a clear target without requiring it to be built now.
- The existing `/ws/browser-view/:id` WebSocket route continues to work via the `VNCStreamingProvider` — no client-breaking change.

## Capabilities

### New Capabilities

- `streaming-provider-interface`: The `StreamingProvider` Go interface and `StreamingSession` model — the contract all streaming backends must implement.
- `vnc-streaming-provider`: Refactored VNC backend implementing `StreamingProvider`; wraps existing x11vnc + noVNC proxy logic; default when `STREAM_BACKEND=vnc`.
- `streaming-session-management`: Per-viewer session lifecycle — create, authenticate, route, and destroy streaming sessions independent of container lifecycle.
- `webrtc-streaming-stub`: Interface contract and placeholder implementation for a future WebRTC backend; documents the signaling flow and ICE negotiation hooks without implementing media transport.

### Modified Capabilities

<!-- No existing specs change at the requirements level — this is a new abstraction layer over current behavior. -->

## Impact

- **Code**: New `internal/streaming/` package (`provider.go`, `session.go`, `vnc_provider.go`, `webrtc_stub.go`); `internal/server/cdp_view.go` and `vnc_proxy.go` refactored behind `VNCStreamingProvider`; `internal/server/workspace_handlers.go` updated to call streaming APIs; `internal/browser/service.go` receives `StreamingProvider` reference.
- **APIs**: Three new endpoints (`/browser/:id/stream/attach`, `/browser/:id/stream/detach`, `/browser/:id/stream/info`); existing `/ws/browser-view/:id` preserved via VNC provider.
- **Config**: New env var `STREAM_BACKEND` (default `vnc`); `STREAM_SESSION_TTL` (default `2h`); `STREAM_MAX_VIEWERS_PER_SESSION` (default `5`).
- **No new external dependencies** for this change — WebRTC media transport (e.g., `pion/webrtc`) is deferred to a follow-up change when the stub is promoted to a full implementation.
- **Frontend**: `app.js` `initAccountScreencast()` updated to use the new attach/detach API; VNC canvas rendering path unchanged.
