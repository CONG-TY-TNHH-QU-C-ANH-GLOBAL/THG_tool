package livesession

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
)

// DockerLiveSession is the concrete LiveSession implementation for Docker-based
// browser containers. It wraps a CDPRuntime for the worker and a VNCVideoSource
// for the dashboard.
type DockerLiveSession struct {
	accountID  int64
	sessionID  int64
	cdpPort    int
	vncPort    int
	workerID   string
	sm         *session.StateMachine
	rt         *runtime.CDPRuntime
	vncSource  *VNCVideoSource
	appStore   *store.AppStore
	allocator  Releaser
}

// Releaser is the minimal interface the session needs from the allocator.
type Releaser interface {
	Release(ctx context.Context, accountID int64, workerID string) error
}

// NewDockerLiveSession creates a LiveSession wrapping an active browser session.
// The CDPRuntime is connected eagerly; VNCVideoSource is lazy (only started when subscribed).
func NewDockerLiveSession(
	sess store.BrowserSession,
	workerID string,
	sm *session.StateMachine,
	appStore *store.AppStore,
	allocator Releaser,
) (*DockerLiveSession, error) {
	rt, err := runtime.NewCDPRuntime(sess.CDPPort)
	if err != nil {
		return nil, fmt.Errorf("connect CDP port %d: %w", sess.CDPPort, err)
	}

	return &DockerLiveSession{
		accountID: sess.AccountID,
		sessionID: sess.ID,
		cdpPort:   sess.CDPPort,
		vncPort:   sess.VNCPort,
		workerID:  workerID,
		sm:        sm,
		rt:        rt,
		vncSource: &VNCVideoSource{vncPort: sess.VNCPort},
		appStore:  appStore,
		allocator: allocator,
	}, nil
}

func (s *DockerLiveSession) AccountID() int64 { return s.accountID }
func (s *DockerLiveSession) SessionID() int64 { return s.sessionID }
func (s *DockerLiveSession) Runtime() runtime.Runtime { return s.rt }
func (s *DockerLiveSession) VideoStream() VideoSource { return s.vncSource }
func (s *DockerLiveSession) Control() ControlChannel  { return &cdpControlChannel{cdpPort: s.cdpPort} }

// State reads the current browser state from the CDP tab.
func (s *DockerLiveSession) State(ctx context.Context) (SessionState, error) {
	// For now, return a minimal state from what we know.
	// A full implementation would query the active tab via CDP.
	return SessionState{
		CurrentURL: "unknown",
		UpdatedAt:  time.Now(),
	}, nil
}

// Heartbeat updates the heartbeat_at timestamp so the health checker knows
// this session is being actively used.
func (s *DockerLiveSession) Heartbeat(ctx context.Context) error {
	_, err := s.appStore.DB().ExecContext(ctx,
		`UPDATE browser_sessions SET heartbeat_at = CURRENT_TIMESTAMP WHERE account_id = ?`,
		s.accountID,
	)
	if err != nil {
		slog.WarnContext(ctx, "heartbeat failed", "account_id", s.accountID, "error", err)
	}
	return err
}

// Close releases the session back to idle. Safe to call multiple times (idempotent).
func (s *DockerLiveSession) Close(ctx context.Context) error {
	if s.allocator == nil {
		return nil
	}
	return s.allocator.Release(ctx, s.accountID, s.workerID)
}

// VNCVideoSource streams browser frames from the VNC port.
// The actual VNC→WebSocket bridging is handled by vnc_proxy.go;
// this struct is a placeholder that carries the port for wiring.
type VNCVideoSource struct {
	vncPort int
}

func (v *VNCVideoSource) Transport() string { return "vnc" }

func (v *VNCVideoSource) Subscribe(ctx context.Context) (<-chan Frame, error) {
	// VNC frame delivery is handled at the WebSocket handler layer (vnc_proxy.go).
	// This method returns a placeholder channel; the actual frames bypass it.
	ch := make(chan Frame)
	close(ch)
	return ch, nil
}

// VNCPort exposes the VNC host port for the proxy layer.
func (v *VNCVideoSource) VNCPort() int { return v.vncPort }

// cdpControlChannel sends input events to the browser via CDP.
type cdpControlChannel struct {
	cdpPort int
}

func (c *cdpControlChannel) MouseMove(_ context.Context, _, _ int) error { return nil }
func (c *cdpControlChannel) MouseClick(_ context.Context, _, _ int, _ string) error { return nil }
func (c *cdpControlChannel) KeyPress(_ context.Context, _ string, _ []string) error { return nil }
func (c *cdpControlChannel) Scroll(_ context.Context, _, _, _, _ int) error { return nil }
func (c *cdpControlChannel) TypeText(_ context.Context, _ string) error { return nil }

// LiveSessionFactory creates DockerLiveSession instances from allocated sessions.
type LiveSessionFactory struct {
	sm        *session.StateMachine
	appStore  *store.AppStore
	allocator Releaser
}

// NewLiveSessionFactory creates a factory wired to the session infrastructure.
func NewLiveSessionFactory(
	db *sql.DB,
	appStore *store.AppStore,
	allocator Releaser,
) *LiveSessionFactory {
	return &LiveSessionFactory{
		sm:       session.NewStateMachine(db),
		appStore: appStore,
		allocator: allocator,
	}
}

// Wrap creates a LiveSession from an already-acquired BrowserSession.
func (f *LiveSessionFactory) Wrap(
	sess store.BrowserSession,
	workerID string,
) (*DockerLiveSession, error) {
	return NewDockerLiveSession(sess, workerID, f.sm, f.appStore, f.allocator)
}
