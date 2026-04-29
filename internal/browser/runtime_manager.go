package browser

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/thg/scraper/internal/agentloop"
	"github.com/thg/scraper/internal/workspace"
)

// SessionOutcome classifies the result of a session health check.
type SessionOutcome string

const (
	SessionOK         SessionOutcome = "OK"
	SessionExpired    SessionOutcome = "EXPIRED"    // login page / cookies gone
	SessionCheckpoint SessionOutcome = "CHECKPOINT" // security challenge — needs human
	SessionBlocked    SessionOutcome = "BLOCKED"    // soft/hard ban
	SessionCDPDown    SessionOutcome = "CDP_DOWN"   // Chrome container unreachable
)

// UnhealthyFunc is called when a session is found to be unhealthy.
// Runs on the goroutine that detected the failure.
// The callee decides what to do: restart, escalate, suspend account, etc.
type UnhealthyFunc func(accountID int64, outcome SessionOutcome, reason string)

// Watchdog periodically checks the Facebook session health for every running
// workspace instance via CDP, and calls onUnhealthy for any that fail.
//
// One Watchdog per application; it iterates all running instances each tick.
type Watchdog struct {
	mgr         workspace.ManagerIface
	interval    time.Duration
	onUnhealthy UnhealthyFunc
}

// NewWatchdog creates a Watchdog. interval should be 30s for production.
func NewWatchdog(mgr workspace.ManagerIface, interval time.Duration, onUnhealthy UnhealthyFunc) *Watchdog {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Watchdog{mgr: mgr, interval: interval, onUnhealthy: onUnhealthy}
}

// Run starts the watchdog loop. Blocks until ctx is cancelled.
// Call this in its own goroutine: go watchdog.Run(ctx).
func (w *Watchdog) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	slog.Info("browser watchdog started", "interval", w.interval)

	for {
		select {
		case <-ctx.Done():
			slog.Info("browser watchdog stopped")
			return
		case <-ticker.C:
			for _, inst := range w.mgr.List() {
				inst := inst // capture for goroutine
				go w.checkInstance(ctx, inst)
			}
		}
	}
}

func (w *Watchdog) checkInstance(ctx context.Context, inst *workspace.Instance) {
	if inst.CDPPort == 0 {
		return
	}
	if time.Since(inst.StartedAt) < 2*time.Minute {
		return
	}

	logger := slog.With("account_id", inst.AccountID, "cdp_port", inst.CDPPort)

	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	checker := agentloop.NewCDPSessionChecker(inst.CDPPort)
	state, err := checker.Check(checkCtx)
	if err != nil {
		logger.Warn("watchdog: CDP unreachable",
			"error", err,
			"outcome", string(SessionCDPDown),
		)
		w.onUnhealthy(inst.AccountID, SessionCDPDown, fmt.Sprintf("CDP unreachable: %v", err))
		return
	}

	ok, reason := state.IsSessionHealthy("")
	if ok {
		logger.Debug("watchdog: session healthy", "url", state.URL)
		return
	}

	outcome := classifyOutcome(state)
	logger.Warn("watchdog: session unhealthy",
		"outcome", string(outcome),
		"reason", reason,
		"url", state.URL,
		"has_checkpoint", state.HasCheckpoint,
		"is_login_page", state.IsLoginPage,
		"is_blocked", state.IsBlocked,
		"cookie_count", state.CookieCount,
	)
	w.onUnhealthy(inst.AccountID, outcome, reason)
}

func classifyOutcome(s *agentloop.FBSessionState) SessionOutcome {
	switch {
	case s.HasCheckpoint:
		return SessionCheckpoint
	case s.IsBlocked:
		return SessionBlocked
	default:
		return SessionExpired // login page or no cookies
	}
}

// SafeRestart stops and restarts the Docker container for accountID without
// touching the Chrome profile directory (session cookies are preserved on disk).
//
// Use this for transient Chrome crashes. Do NOT call this when the session is
// expired (IsLoginPage=true) — the profile is intact but the account needs a
// manual re-login via the browser UI.
func SafeRestart(ctx context.Context, mgr workspace.ManagerIface, accountID int64, accountName string) error {
	slog.Info("browser: safe restart initiated",
		"account_id", accountID,
		"account_name", accountName,
	)

	mgr.Stop(accountID)

	// Wait for Docker to finish cleanup before launching a new container.
	select {
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}

	if _, err := mgr.Start(accountID, accountName); err != nil {
		slog.Error("browser: safe restart failed",
			"account_id", accountID,
			"error", err,
		)
		return fmt.Errorf("safe restart account %d: %w", accountID, err)
	}

	slog.Info("browser: safe restart complete", "account_id", accountID)
	return nil
}
