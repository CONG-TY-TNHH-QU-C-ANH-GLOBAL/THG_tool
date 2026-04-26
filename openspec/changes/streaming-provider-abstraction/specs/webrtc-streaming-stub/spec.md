## ADDED Requirements

### Requirement: WebRTC stub implements StreamingProvider interface
The system SHALL provide a `WebRTCStreamingStub` struct that implements all `StreamingProvider` interface methods. Every method SHALL return `streaming.ErrNotImplemented` (HTTP 501 at the API layer). The stub SHALL be selectable via `STREAM_BACKEND=webrtc`.

#### Scenario: WebRTC attach returns not-implemented error
- **WHEN** `STREAM_BACKEND=webrtc` and a container starts
- **THEN** `WebRTCStreamingStub.Attach` is called; it returns `streaming.ErrNotImplemented`; `DockerBrowserService.Start` logs a WARN and the container start fails with a clear error message indicating WebRTC is not yet implemented

#### Scenario: WebRTC session attach API returns 501
- **WHEN** `STREAM_BACKEND=webrtc` and `POST /browser/:id/stream/attach` is called
- **THEN** the response is HTTP 501 with `{ "error": "WebRTC streaming is not yet implemented" }`

### Requirement: WebRTC stub documents signaling contract in code
The `WebRTCStreamingStub` struct's field definitions and method doc-comments SHALL explicitly document the expected WebRTC signaling flow, including: what `Attach` must do (start a GStreamer or FFmpeg pipeline from the container's display to a WebRTC media source), what `NewSession` must do (create a PeerConnection, return an SDP offer in `SessionDescriptor.SDPOffer`), and what ICE server configuration is needed.

#### Scenario: Stub struct is self-documenting
- **WHEN** a developer reads `WebRTCStreamingStub` source
- **THEN** they can understand the required STUN/TURN server fields, the SDP offer/answer flow, and the ICE candidate exchange mechanism without reading the design doc

### Requirement: SessionDescriptor is extensible for WebRTC
The `SessionDescriptor` struct returned by `StreamingProvider.Describe` SHALL include optional fields `SDPOffer string` and `ICEServers []ICEServer` that are empty for VNC sessions and populated by a full WebRTC implementation.

#### Scenario: VNC Describe omits WebRTC fields
- **WHEN** `VNCStreamingProvider.Describe(ctx, sessionID)` is called
- **THEN** the returned `SessionDescriptor` has `SDPOffer: ""` and `ICEServers: nil`; `ConnectURL` is the noVNC WebSocket path

#### Scenario: WebRTC Describe populates SDP fields
- **WHEN** a full (non-stub) `WebRTCStreamingProvider.Describe` is called (future implementation)
- **THEN** the `SessionDescriptor` has a non-empty `SDPOffer` and at least one `ICEServer` entry; `ConnectURL` is empty (WebRTC uses SDP not a URL)
