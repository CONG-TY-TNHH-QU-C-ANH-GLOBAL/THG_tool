## ADDED Requirements

### Requirement: StreamingProvider interface contract
The system SHALL define a `StreamingProvider` Go interface in `internal/streaming/provider.go` that all streaming backends must implement. The interface SHALL include `Attach`, `Detach`, `NewSession`, `CloseSession`, `ListSessions`, and `Describe` methods. `BrowserService` SHALL depend only on this interface, never on a concrete implementation.

#### Scenario: VNC provider satisfies interface at compile time
- **WHEN** the project is compiled
- **THEN** `VNCStreamingProvider` satisfies `StreamingProvider` with no type assertion failures

#### Scenario: WebRTC stub satisfies interface at compile time
- **WHEN** the project is compiled
- **THEN** `WebRTCStreamingStub` satisfies `StreamingProvider` with no type assertion failures

#### Scenario: BrowserService works with any provider
- **WHEN** `BrowserService` is constructed with a `StreamingProvider` parameter
- **THEN** it calls only interface methods; substituting a different provider requires no change to `BrowserService`

### Requirement: Provider attached after container start
The system SHALL call `StreamingProvider.Attach(ctx, ContainerAttachInfo)` immediately after a Docker container starts successfully. `ContainerAttachInfo` SHALL include `AccountID`, `ContainerID`, `VNCPort`, `CDPPort`, and `DisplayNum`. `Attach` SHALL return a `StreamingEndpoint` describing the active transport.

#### Scenario: Attach called on successful container start
- **WHEN** `DockerBrowserService.Start` completes successfully for account 42
- **THEN** `provider.Attach(ctx, {AccountID: 42, VNCPort: <port>, ...})` is called and the returned `StreamingEndpoint` is stored in the container's `ContainerInfo`

#### Scenario: Attach failure causes Start to fail
- **WHEN** `provider.Attach` returns an error after container creation
- **THEN** `DockerBrowserService.Start` stops and removes the container, releases ports, and returns the error to the caller

### Requirement: Provider detached before container stop
The system SHALL call `StreamingProvider.Detach(ctx, accountID)` before a Docker container is stopped. `Detach` SHALL close all active viewer sessions for the account and tear down the streaming transport.

#### Scenario: Detach called before container removal
- **WHEN** `DockerBrowserService.Stop` is called for account 42
- **THEN** `provider.Detach(ctx, 42)` is called first; all sessions for account 42 are closed; then the Docker container is stopped and removed

#### Scenario: Detach error does not block container stop
- **WHEN** `provider.Detach` returns an error
- **THEN** the error is logged at WARN and the container stop proceeds regardless

### Requirement: Backend selection via environment variable
The system SHALL select the active `StreamingProvider` implementation based on `STREAM_BACKEND` env var (`vnc` or `webrtc`). An unknown value SHALL cause startup failure with a clear error.

#### Scenario: VNC selected by default
- **WHEN** `STREAM_BACKEND` is not set
- **THEN** `VNCStreamingProvider` is instantiated as the active backend

#### Scenario: Unknown backend causes startup failure
- **WHEN** `STREAM_BACKEND=grpc` is set
- **THEN** the service fails to start with an error listing valid options: `vnc`, `webrtc`
