// Package accountsafety is the pure-policy foundation for the Facebook crawler
// Account Safety Coordinator (spec: specs/facebook/CRAWLER_ACCOUNT_SAFETY_SPEC.md).
//
// It answers "which account may safely crawl now, and what happens after a run"
// with side-effect-free functions over an explicit MachineState. It holds NO
// state, opens NO connections, and performs NO I/O — the runtime holder + lease
// wiring (reusing session.Allocator) land in a focused follow-up.
//
// SAFETY INVARIANTS (binding):
//   - Default one active crawl per machine; one workflow per account.
//   - checkpoint_suspected / login_required / risk_blocked → human-required-class
//     states that NEVER auto-clear by timer; only an operator/verifier path
//     (ResolveHumanRequired) clears them. No auto-retry, no account rotation to
//     dodge a wall, no checkpoint bypass.
//   - cooling_down is a TIME-based pacing state (clean runs only) that expires.
package accountsafety

import "time"

// Status is an account's crawler-runtime status (layered above the browser
// session.Status; checkpoint/login/risk map onto the session checkpoint lifecycle).
type Status string

const (
	StatusReady              Status = "ready"
	StatusQueued             Status = "queued"
	StatusRunning            Status = "running"
	StatusCoolingDown        Status = "cooling_down"
	StatusStalledNoProgress  Status = "stalled_no_progress"
	StatusCheckpointRequired Status = "checkpoint_required"
	StatusLoginRequired      Status = "login_required"
	StatusRiskBlocked        Status = "risk_blocked"
	StatusHumanRequired      Status = "human_required"
)

// AccountState is the per-account safety snapshot the policy reasons over.
type AccountState struct {
	AccountID          int64
	Status             Status
	CooldownUntil      time.Time // meaningful for StatusCoolingDown / StatusStalledNoProgress
	LastSafeStopReason string    // the exit_reason of the last non-clean stop
	QueuedAt           time.Time // FIFO ordering key for queued/ready accounts
}

// Config carries the safe defaults. Conservative by construction — tuning is a
// later, evidence-driven change; nothing here raises concurrency.
type Config struct {
	MaxActiveCrawlsPerMachine int           // hard cap on concurrent crawls on this host
	CleanRunCooldown          time.Duration // optional pacing gap after a clean run (0 = none)
	// StalledNoProgressCooldown is the optional machine-level backoff after a
	// stalled/no-progress stop. Default 0: the coordinator adds no artificial
	// backoff beyond the recurring-intent interval (which already gates re-runs)
	// and the FIFO queue (which stops a stalled account starving others) — so no
	// retry storm. Eligibility semantics are identical to cooling_down (skipped
	// until cooldown_until), so setting a positive value later — once telemetry
	// justifies it — yields machine-level stall backoff with no other change.
	StalledNoProgressCooldown time.Duration
}

// DefaultConfig is the binding safe default: exactly one active crawl per machine,
// no artificial post-run backoff (see StalledNoProgressCooldown).
func DefaultConfig() Config {
	return Config{MaxActiveCrawlsPerMachine: 1, CleanRunCooldown: 0, StalledNoProgressCooldown: 0}
}

// MachineState is the host-scoped view the policy reads (a value, never mutated).
type MachineState struct {
	Config   Config
	Accounts []AccountState
}

// activeCount is the number of accounts currently running on this machine.
func (m MachineState) activeCount() int {
	n := 0
	for _, a := range m.Accounts {
		if a.Status == StatusRunning {
			n++
		}
	}
	return n
}

// IsHumanRequired reports states that require an operator/verifier and must
// NEVER be cleared by a timer (the safety-critical no-auto-resume invariant).
func IsHumanRequired(s Status) bool {
	switch s {
	case StatusCheckpointRequired, StatusLoginRequired, StatusRiskBlocked, StatusHumanRequired:
		return true
	default:
		return false
	}
}
