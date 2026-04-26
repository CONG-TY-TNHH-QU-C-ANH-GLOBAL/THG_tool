// Package livesession defines the unified interface through which both the worker
// and the dashboard interact with a running browser session.
// Workers use Runtime() to scrape; the dashboard uses VideoStream() to view the browser.
package livesession

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/runtime"
)

// LiveSession is the single interaction point for a running browser session.
// All access — worker automation, dashboard VNC, control input — goes through here.
type LiveSession interface {
	AccountID() int64
	SessionID() int64

	// Runtime returns a Runtime bound to this session's CDP port.
	// Used by the worker to scrape Facebook.
	Runtime() runtime.Runtime

	// VideoStream returns a VideoSource for streaming browser frames to the dashboard.
	VideoStream() VideoSource

	// Control returns a channel for forwarding human/agent input to the browser.
	Control() ControlChannel

	// State returns the current browser state (URL, login status, active tab).
	State(ctx context.Context) (SessionState, error)

	// Heartbeat updates the session's heartbeat_at timestamp in the DB.
	// Workers call this every 30s while a job is active.
	Heartbeat(ctx context.Context) error

	// Close releases the session back to idle without terminating the container.
	Close(ctx context.Context) error
}

// VideoSource delivers browser frames to the dashboard via a channel.
type VideoSource interface {
	// Subscribe returns a channel of JPEG frames until ctx is cancelled.
	Subscribe(ctx context.Context) (<-chan Frame, error)
	// Transport identifies the delivery mechanism: "vnc" or "cdp_screencast".
	Transport() string
}

// ControlChannel forwards human or agent input into the browser.
type ControlChannel interface {
	MouseMove(ctx context.Context, x, y int) error
	MouseClick(ctx context.Context, x, y int, button string) error
	KeyPress(ctx context.Context, key string, mods []string) error
	Scroll(ctx context.Context, x, y, deltaX, deltaY int) error
	TypeText(ctx context.Context, text string) error
}

// Frame is a single rendered browser frame.
type Frame struct {
	Data       []byte
	Width      int
	Height     int
	CapturedAt time.Time
}

// SessionState captures the observable state of the browser at a point in time.
type SessionState struct {
	CurrentURL string
	LoggedIn   bool
	TabID      string
	Title      string
	UpdatedAt  time.Time
}
