package accountsafety

import "time"

// Crawl exit_reason constants (mirror the values the crawler emits — risk codes
// from platforms/facebook/crawl_progress.js, stall/exhaustion codes from
// crawl_pacing.js / crawl.js). Centralized here so the classifier has no magic
// strings. Clean/success reasons (maxItems, completed, cursor_match, "") are the
// default branch — no constant needed.
const (
	ReasonCheckpointSuspected   = "checkpoint_suspected"
	ReasonLoginRequired         = "login_required"
	ReasonRiskBlocked           = "risk_blocked"
	ReasonNoProgress            = "no_progress"
	ReasonNoNewItemsAfterScroll = "no_new_items_after_scroll"
	ReasonDuplicateHeavy        = "duplicate_heavy"
	ReasonScrollNotMoving       = "scroll_not_moving"
	ReasonPassExhausted         = "pass_exhausted"
)

// EvaluateRisk maps a crawl exit_reason to the resulting account Status and
// whether it is a risk stop. Only the three explicit risk reasons are
// human-required-class. Non-risk reasons return (StatusReady, false) here; the
// stalled vs clean distinction is drawn by IsStalledReason / ApplyStop.
func EvaluateRisk(exitReason string) (Status, bool) {
	switch exitReason {
	case ReasonCheckpointSuspected:
		return StatusCheckpointRequired, true
	case ReasonLoginRequired:
		return StatusLoginRequired, true
	case ReasonRiskBlocked:
		return StatusRiskBlocked, true
	default:
		return StatusReady, false
	}
}

// IsStalledReason reports a non-risk stop where the crawl made no real progress
// (feed not loading, only duplicates, scroll stuck, or pass budget spent). These
// are distinct from a clean/success finish and map to StatusStalledNoProgress.
func IsStalledReason(exitReason string) bool {
	switch exitReason {
	case ReasonNoProgress, ReasonNoNewItemsAfterScroll, ReasonDuplicateHeavy,
		ReasonScrollNotMoving, ReasonPassExhausted:
		return true
	default:
		return false
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
// NOT time-clear (human-required). Stalled stops use StalledNoProgressCooldown;
// clean stops use CleanRunCooldown.
func CooldownDuration(exitReason string, cfg Config) time.Duration {
	if _, isRisk := EvaluateRisk(exitReason); isRisk {
		return 0
	}
	if IsStalledReason(exitReason) {
		return cfg.StalledNoProgressCooldown
	}
	return cfg.CleanRunCooldown
}

// addOrZero returns now+d for a positive d, else the zero time (no timer).
func addOrZero(now time.Time, d time.Duration) time.Time {
	if d > 0 {
		return now.Add(d)
	}
	return time.Time{}
}

// ApplyStop is the pure transition of an account after a crawl run finishes:
//   - risk    → the matching human-required-class state, NO cooldown timer;
//   - stalled → stalled_no_progress with the (optional) stalled cooldown;
//   - clean   → cooling_down (if a pacing gap is configured) else ready.
func ApplyStop(st AccountState, exitReason string, now time.Time, cfg Config) AccountState {
	if status, isRisk := EvaluateRisk(exitReason); isRisk {
		st.Status = status
		st.CooldownUntil = time.Time{} // human-required: never auto-clears
		st.LastSafeStopReason = exitReason
		return st
	}
	if IsStalledReason(exitReason) {
		st.Status = StatusStalledNoProgress
		st.CooldownUntil = addOrZero(now, cfg.StalledNoProgressCooldown)
		st.LastSafeStopReason = exitReason
		return st
	}
	// Clean / success finish.
	st.LastSafeStopReason = ""
	if cfg.CleanRunCooldown > 0 {
		st.Status = StatusCoolingDown
		st.CooldownUntil = now.Add(cfg.CleanRunCooldown)
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
	case StatusCoolingDown, StatusStalledNoProgress:
		// Time-based states: eligible once the (possibly zero) cooldown elapses.
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
