package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/thg/scraper/internal/observability"
)

type cbStatus int

const (
	cbClosed   cbStatus = iota
	cbOpen
	cbHalfOpen
)

// cbState tracks the circuit breaker state for one scope.
type cbState struct {
	state      cbStatus
	failures   int
	firstFail  time.Time
	opensUntil time.Time
}

// CircuitBreaker prevents restart storms at both per-account and global levels.
// Per-account: 3 failures within 5min → open (60s cooldown).
// Global: 5 different accounts fail within 60s → open (5min cooldown).
type CircuitBreaker struct {
	mu         sync.Mutex
	perAccount map[int64]*cbState
	global     *cbState
	db         *sql.DB
	alertFn    func(msg string) // optional Telegram/webhook alert hook

	// Config
	perFailThreshold int           // 3
	perWindow        time.Duration // 5min
	perCooldown      time.Duration // 60s

	globalFailThreshold int           // 5
	globalWindow        time.Duration // 60s
	globalCooldown      time.Duration // 5min
}

// NewCircuitBreaker creates a circuit breaker with production defaults.
func NewCircuitBreaker(db *sql.DB, alertFn func(msg string)) *CircuitBreaker {
	return &CircuitBreaker{
		perAccount:          make(map[int64]*cbState),
		global:              &cbState{},
		db:                  db,
		alertFn:             alertFn,
		perFailThreshold:    3,
		perWindow:           5 * time.Minute,
		perCooldown:         60 * time.Second,
		globalFailThreshold: 5,
		globalWindow:        60 * time.Second,
		globalCooldown:      5 * time.Minute,
	}
}

// AllowRestart reports whether a restart for accountID is currently permitted.
func (cb *CircuitBreaker) AllowRestart(accountID int64) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	// Check global breaker first
	if !cb.checkAllow(cb.global, now) {
		observability.CircuitBreakerOpen.WithLabelValues("global").Set(1)
		return false
	}
	observability.CircuitBreakerOpen.WithLabelValues("global").Set(0)

	// Check per-account breaker
	if state, ok := cb.perAccount[accountID]; ok {
		if !cb.checkAllow(state, now) {
			observability.CircuitBreakerOpen.WithLabelValues(fmt.Sprintf("account:%d", accountID)).Set(1)
			return false
		}
		observability.CircuitBreakerOpen.WithLabelValues(fmt.Sprintf("account:%d", accountID)).Set(0)
	}

	return true
}

func (cb *CircuitBreaker) checkAllow(state *cbState, now time.Time) bool {
	switch state.state {
	case cbClosed:
		return true
	case cbOpen:
		if now.After(state.opensUntil) {
			state.state = cbHalfOpen
			return true
		}
		return false
	case cbHalfOpen:
		return true // allow one trial
	}
	return true
}

// RecordFailure records a failure for accountID and potentially opens the breaker.
func (cb *CircuitBreaker) RecordFailure(accountID int64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	// Per-account tracking
	state, ok := cb.perAccount[accountID]
	if !ok {
		state = &cbState{}
		cb.perAccount[accountID] = state
	}
	if state.failures == 0 || now.Sub(state.firstFail) > cb.perWindow {
		state.failures = 0
		state.firstFail = now
	}
	state.failures++
	if state.failures >= cb.perFailThreshold {
		state.state = cbOpen
		state.opensUntil = now.Add(cb.perCooldown)
		slog.Warn("circuit breaker opened for account",
			"account_id", accountID,
			"failures", state.failures,
			"reopens_at", state.opensUntil,
		)
		observability.CircuitBreakerOpen.WithLabelValues(fmt.Sprintf("account:%d", accountID)).Set(1)
		cb.persistState(fmt.Sprintf("account:%d", accountID), "open", state.opensUntil)
	}

	// Global tracking
	if cb.global.failures == 0 || now.Sub(cb.global.firstFail) > cb.globalWindow {
		cb.global.failures = 0
		cb.global.firstFail = now
	}
	cb.global.failures++
	if cb.global.failures >= cb.globalFailThreshold {
		cb.global.state = cbOpen
		cb.global.opensUntil = now.Add(cb.globalCooldown)
		msg := fmt.Sprintf("🚨 GLOBAL CIRCUIT BREAKER OPEN: %d container failures in 60s. All restarts paused for 5min.", cb.global.failures)
		slog.Error(msg)
		if cb.alertFn != nil {
			cb.alertFn(msg)
		}
		observability.CircuitBreakerOpen.WithLabelValues("global").Set(1)
		cb.persistState("global", "open", cb.global.opensUntil)
	}
}

// RecordSuccess resets the per-account failure counter and closes the breaker.
func (cb *CircuitBreaker) RecordSuccess(accountID int64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if state, ok := cb.perAccount[accountID]; ok {
		state.state = cbClosed
		state.failures = 0
		observability.CircuitBreakerOpen.WithLabelValues(fmt.Sprintf("account:%d", accountID)).Set(0)
		cb.persistState(fmt.Sprintf("account:%d", accountID), "closed", time.Time{})
	}
}

func (cb *CircuitBreaker) persistState(scope, state string, opensUntil time.Time) {
	if cb.db == nil {
		return
	}
	var until any
	if !opensUntil.IsZero() {
		until = opensUntil.UTC().Format(time.RFC3339)
	}
	cb.db.Exec(`INSERT OR REPLACE INTO circuit_breaker_state (scope, state, opens_until, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)`, scope, state, until)
}

// RestartController watches for unhealthy sessions and applies the restart policy
// with circuit breaker protection.
//
// Concurrency model: every OnUnhealthy call serialises through restarting,
// a per-account boolean. The HealthChecker is single-threaded today but
// other components (the optional watchdog, manual /restart endpoint,
// external observability hooks) can also signal "restart this account".
// Without the guard, a slow Docker stop/start that takes longer than the
// 15 s health tick would let the next tick re-enter and start a second
// stop/start while the first is still mid-flight.
type RestartController struct {
	mgr        ManagerIface
	cb         *CircuitBreaker
	maxRetries int

	mu         sync.Mutex
	restarting map[int64]time.Time // accountID → time we started restarting
}

// NewRestartController creates a controller with a max of 3 restart attempts per account.
func NewRestartController(mgr ManagerIface, cb *CircuitBreaker) *RestartController {
	return &RestartController{
		mgr:        mgr,
		cb:         cb,
		maxRetries: 3,
		restarting: make(map[int64]time.Time),
	}
}

// markRestarting reserves the per-account restart slot. Returns false when
// another goroutine is already restarting this account (or recently did).
// The caller must call clearRestarting in a defer when it returns true.
func (rc *RestartController) markRestarting(accountID int64) bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	// Second-level guard: even if a goroutine cleared the flag but the
	// account is in restart cooldown, refuse to restart again immediately.
	const cooldown = 30 * time.Second
	if last, ok := rc.restarting[accountID]; ok {
		if last.IsZero() {
			return false // currently in flight
		}
		if time.Since(last) < cooldown {
			return false // recent finish, debounce
		}
	}
	rc.restarting[accountID] = time.Time{} // zero = in flight
	return true
}

func (rc *RestartController) clearRestarting(accountID int64) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.restarting[accountID] = time.Now()
}

// OnUnhealthy is called by HealthChecker when a container fails a health check.
// It checks the circuit breaker then attempts a container restart with backoff.
func (rc *RestartController) OnUnhealthy(ctx context.Context, accountID int64) {
	if !rc.markRestarting(accountID) {
		slog.DebugContext(ctx, "restart already in flight or cooling down",
			"account_id", accountID)
		return
	}
	defer rc.clearRestarting(accountID)

	if !rc.cb.AllowRestart(accountID) {
		slog.WarnContext(ctx, "circuit breaker blocking restart", "account_id", accountID)
		return
	}

	slog.WarnContext(ctx, "attempting container restart", "account_id", accountID)

	inst := rc.mgr.Get(accountID)
	if inst == nil {
		slog.WarnContext(ctx, "no tracked instance for unhealthy account", "account_id", accountID)
		return
	}

	rc.mgr.Stop(accountID)

	// Brief pause before restart (10s)
	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Second):
	}

	_, err := rc.mgr.Start(accountID, inst.AccountName)
	if err != nil {
		slog.ErrorContext(ctx, "container restart failed",
			"account_id", accountID,
			"error", err,
		)
		rc.cb.RecordFailure(accountID)
		observability.ContainerRestarts.WithLabelValues("failed").Inc()
		return
	}

	rc.cb.RecordSuccess(accountID)
	observability.ContainerRestarts.WithLabelValues("success").Inc()
	slog.InfoContext(ctx, "container restarted successfully", "account_id", accountID)
}
