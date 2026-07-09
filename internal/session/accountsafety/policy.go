package accountsafety

import "time"

// EvaluateRisk maps a crawl exit_reason to the resulting account Status and
// whether it is a risk stop. Only the three explicit risk reasons are
// human-required-class; every other (clean / stall / exhaustion) reason is
// benign and lets the account be scheduled again on its normal interval.
func EvaluateRisk(exitReason string) (Status, bool) {
	switch exitReason {
	case "checkpoint_suspected":
		return StatusCheckpointRequired, true
	case "login_required":
		return StatusLoginRequired, true
	case "risk_blocked":
		return StatusRiskBlocked, true
	default:
		return StatusReady, false
	}
}

// ShouldEnterCooldown reports whether a stop should pace before the next run.
// Only clean stops are cooldown-eligible; risk stops go human-required instead
// (they must not silently resume after a timer).
func ShouldEnterCooldown(exitReason string) bool {
	_, isRisk := EvaluateRisk(exitReason)
	return !isRisk
}

// CooldownDuration is the timed pause for a stop. Risk stops return 0 — they do
// NOT time-clear (human-required). Clean stops use the configured pacing gap.
func CooldownDuration(exitReason string, cfg Config) time.Duration {
	if !ShouldEnterCooldown(exitReason) {
		return 0
	}
	return cfg.CleanRunCooldown
}

// ApplyStop is the pure transition of an account after a crawl run finishes.
// Risk → the matching human-required-class state with NO cooldown timer. Clean →
// cooling_down (if a pacing gap is configured) else ready.
func ApplyStop(st AccountState, exitReason string, now time.Time, cfg Config) AccountState {
	st.LastSafeStopReason = exitReason
	if status, isRisk := EvaluateRisk(exitReason); isRisk {
		st.Status = status
		st.CooldownUntil = time.Time{} // human-required: never auto-clears
		return st
	}
	if d := CooldownDuration(exitReason, cfg); d > 0 {
		st.Status = StatusCoolingDown
		st.CooldownUntil = now.Add(d)
		return st
	}
	st.Status = StatusReady
	st.CooldownUntil = time.Time{}
	return st
}

// ResolveHumanRequired is the ONLY path out of a human-required-class state:
// an explicit operator/verifier action (wired later to CheckpointManager). It
// never fires on a timer. No-op for any other status.
func ResolveHumanRequired(st AccountState) AccountState {
	if IsHumanRequired(st.Status) {
		st.Status = StatusReady
		st.CooldownUntil = time.Time{}
		st.LastSafeStopReason = ""
	}
	return st
}

// IsEligible reports whether an account may start a crawl now: ready/queued, or
// a cooling_down account whose cooldown has elapsed. Running and every
// human-required-class state are never eligible.
func IsEligible(st AccountState, now time.Time) bool {
	switch st.Status {
	case StatusReady, StatusQueued:
		return true
	case StatusCoolingDown:
		return !now.Before(st.CooldownUntil)
	default:
		return false
	}
}

// CanStartAccount reports whether a specific account may start now: the machine
// budget has a free slot AND the account is eligible.
func CanStartAccount(accountID int64, m MachineState, now time.Time) bool {
	if m.activeCount() >= m.Config.MaxActiveCrawlsPerMachine {
		return false
	}
	for _, a := range m.Accounts {
		if a.AccountID == accountID {
			return IsEligible(a, now)
		}
	}
	return false
}

// NextAccountToRun returns the FIFO-earliest eligible account to start, honoring
// the per-machine budget. ok=false when the budget is full or none is eligible.
func NextAccountToRun(m MachineState, now time.Time) (int64, bool) {
	if m.activeCount() >= m.Config.MaxActiveCrawlsPerMachine {
		return 0, false
	}
	var best AccountState
	found := false
	for _, a := range m.Accounts {
		if !IsEligible(a, now) {
			continue
		}
		if !found || a.QueuedAt.Before(best.QueuedAt) {
			best = a
			found = true
		}
	}
	return best.AccountID, found
}
