## ADDED Requirements

### Requirement: Session creation via attach endpoint
The system SHALL create a `StreamingSession` when `POST /browser/:id/stream/attach` is called for a running container. The session SHALL include a unique `session_id`, a signed JWT viewer token scoped to that session, a `connect_url`, a `protocol` field matching the active backend, and an expiry time of `now + STREAM_SESSION_TTL`.

#### Scenario: Session created for running container
- **WHEN** `POST /browser/:id/stream/attach` is called for account 42 with a running container
- **THEN** the response is HTTP 201 with `{ "session_id", "connect_url", "protocol": "vnc", "expires_at", "viewer_token" }`

#### Scenario: Session rejected when no container running
- **WHEN** `POST /browser/:id/stream/attach` is called for an account with no running container
- **THEN** the response is HTTP 404 with `{ "error": "no running container for account" }`

#### Scenario: Session rejected when max viewers reached
- **WHEN** `POST /browser/:id/stream/attach` is called and the account already has `STREAM_MAX_VIEWERS_PER_SESSION` active sessions
- **THEN** the response is HTTP 429 with `{ "error": "max viewers reached", "limit": STREAM_MAX_VIEWERS_PER_SESSION }`

### Requirement: Session termination via detach endpoint
The system SHALL close a `StreamingSession` when `DELETE /browser/:id/stream/detach?session_id=<id>` is called, sending a WebSocket close frame to the viewer and removing the session from the registry.

#### Scenario: Session closed on detach
- **WHEN** `DELETE /browser/:id/stream/detach?session_id=<id>` is called for a valid session
- **THEN** the session is removed from the registry, the viewer's WebSocket receives a close frame (code 1000), and the response is HTTP 200 with `{ "session_id", "status": "closed" }`

#### Scenario: Detach for unknown session
- **WHEN** `DELETE /browser/:id/stream/detach?session_id=unknown` is called
- **THEN** the response is HTTP 404 with `{ "error": "session not found" }`

### Requirement: Session info endpoint
The system SHALL expose `GET /browser/:id/stream/info` returning all active sessions for the account, each with `session_id`, `protocol`, `connect_url`, `expires_at`, and `viewer_id`.

#### Scenario: Active sessions listed
- **WHEN** `GET /browser/:id/stream/info` is called for account 42 with 2 active sessions
- **THEN** the response is HTTP 200 with `{ "account_id": 42, "protocol": "vnc", "sessions": [{ "session_id", "viewer_id", "connect_url", "expires_at" }, ...] }`

#### Scenario: No active sessions
- **WHEN** `GET /browser/:id/stream/info` is called for an account with no sessions
- **THEN** the response is `{ "account_id": 42, "sessions": [] }`

### Requirement: Session viewer token validation
The system SHALL validate the `viewer_token` JWT on every WebSocket upgrade to `/ws/browser-view/:id`. Upgrades without a valid, unexpired token SHALL be rejected when `STREAM_REQUIRE_SESSION_TOKEN=true`.

#### Scenario: Valid token allows WebSocket upgrade
- **WHEN** a WebSocket upgrade request to `/ws/browser-view/42` includes a valid viewer token for session 42
- **THEN** the upgrade proceeds and the viewer receives the stream

#### Scenario: Missing token rejected when enforcement enabled
- **WHEN** `STREAM_REQUIRE_SESSION_TOKEN=true` and a WebSocket upgrade request has no `viewer_token`
- **THEN** the server responds HTTP 401 and does not upgrade

#### Scenario: Token enforcement disabled for transition period
- **WHEN** `STREAM_REQUIRE_SESSION_TOKEN=false` (default)
- **THEN** WebSocket upgrades without a viewer token are allowed (backward compatibility)

### Requirement: Automatic session expiry
The system SHALL run a background reaper that closes and removes sessions whose `expires_at` has passed, sending a WebSocket close frame (code 1001 — Going Away) to active viewers.

#### Scenario: Expired session reclaimed
- **WHEN** a session's `expires_at` passes and the reaper ticks
- **THEN** the session is removed from the registry and the viewer's WebSocket receives close frame 1001

#### Scenario: All sessions closed on container stop
- **WHEN** `DockerBrowserService.Stop` triggers `provider.Detach` for account 42
- **THEN** all sessions for account 42 are immediately closed regardless of their `expires_at`, and viewers receive close frame 1001
