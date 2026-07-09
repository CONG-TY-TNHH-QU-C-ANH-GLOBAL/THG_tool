package accountsafety

import (
	"testing"
	"time"
)

var testNow = time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)

// #1 default machine budget allows only 1 running account.
func TestMachineBudgetOne(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxActiveCrawlsPerMachine != 1 {
		t.Fatalf("default budget must be 1, got %d", cfg.MaxActiveCrawlsPerMachine)
	}
	m := MachineState{Config: cfg, Accounts: []AccountState{
		{AccountID: 1, Status: StatusRunning},
		{AccountID: 2, Status: StatusReady},
	}}
	if _, ok := NextAccountToRun(m, testNow); ok {
		t.Error("budget full (1 running) → NextAccountToRun must return none")
	}
	if CanStartAccount(2, m, testNow) {
		t.Error("budget full → account 2 must not be startable")
	}
}

// #2 same account cannot run two workflows (already running → not startable).
func TestSameAccountNoDoubleRun(t *testing.T) {
	m := MachineState{Config: Config{MaxActiveCrawlsPerMachine: 5}, Accounts: []AccountState{
		{AccountID: 1, Status: StatusRunning},
	}}
	if CanStartAccount(1, m, testNow) {
		t.Error("a running account must not start a second workflow")
	}
	if IsEligible(AccountState{Status: StatusRunning}, testNow) {
		t.Error("running account must never be eligible")
	}
}

// #3 FIFO selection chooses the earliest eligible account.
func TestFIFOSelection(t *testing.T) {
	m := MachineState{Config: DefaultConfig(), Accounts: []AccountState{
		{AccountID: 10, Status: StatusQueued, QueuedAt: testNow.Add(2 * time.Minute)},
		{AccountID: 11, Status: StatusQueued, QueuedAt: testNow.Add(1 * time.Minute)}, // earliest
		{AccountID: 12, Status: StatusQueued, QueuedAt: testNow.Add(3 * time.Minute)},
	}}
	got, ok := NextAccountToRun(m, testNow)
	if !ok || got != 11 {
		t.Errorf("FIFO must pick earliest QueuedAt (11), got %d ok=%v", got, ok)
	}
}

// #4 cooling_down account is skipped until cooldown_until, then eligible.
func TestCooldownExpiry(t *testing.T) {
	cooling := AccountState{AccountID: 1, Status: StatusCoolingDown, CooldownUntil: testNow.Add(5 * time.Minute)}
	if IsEligible(cooling, testNow) {
		t.Error("cooling_down before cooldown_until must be skipped")
	}
	if !IsEligible(cooling, testNow.Add(5*time.Minute)) {
		t.Error("cooling_down at/after cooldown_until must be eligible")
	}
}

// #5 checkpoint_suspected/login_required/risk_blocked do not auto-clear by time.
func TestRiskStatesDoNotAutoClear(t *testing.T) {
	farFuture := testNow.Add(1000 * time.Hour)
	for _, reason := range []string{ReasonCheckpointSuspected, ReasonLoginRequired, ReasonRiskBlocked} {
		st := ApplyStop(AccountState{AccountID: 1}, reason, testNow, DefaultConfig())
		if !IsHumanRequired(st.Status) {
			t.Errorf("%s must map to a human-required-class status, got %s", reason, st.Status)
		}
		if !st.CooldownUntil.IsZero() {
			t.Errorf("%s must not set a cooldown timer, got %v", reason, st.CooldownUntil)
		}
		if IsEligible(st, farFuture) {
			t.Errorf("%s must never become eligible by time", reason)
		}
	}
}

// #6 human_required is never cleared by time; only ResolveHumanRequired clears it.
func TestHumanRequiredNeverTimeCleared(t *testing.T) {
	st := AccountState{AccountID: 1, Status: StatusHumanRequired}
	if IsEligible(st, testNow.Add(10000*time.Hour)) {
		t.Error("human_required must never be eligible by time")
	}
	resolved := ResolveHumanRequired(st)
	if resolved.Status != StatusReady {
		t.Errorf("operator resolve must return to ready, got %s", resolved.Status)
	}
	if !IsEligible(resolved, testNow) {
		t.Error("resolved account must be eligible")
	}
}

// #7 a risk exit reason triggers a non-auto-clearing human-required state.
func TestRiskExitTriggersBlock(t *testing.T) {
	st := ApplyStop(AccountState{AccountID: 1, Status: StatusRunning}, ReasonRiskBlocked, testNow, DefaultConfig())
	if st.Status != StatusRiskBlocked {
		t.Errorf("risk_blocked exit → risk_blocked status, got %s", st.Status)
	}
	if st.LastSafeStopReason != ReasonRiskBlocked {
		t.Errorf("last stop reason must be recorded, got %q", st.LastSafeStopReason)
	}
	if _, isRisk := EvaluateRisk(ReasonRiskBlocked); !isRisk {
		t.Error("risk_blocked must classify as risk")
	}
}

// #8 a clean/completed crawl releases the account (→ ready) so the next queued
// account can run once the running one finishes.
func TestCleanFinishAllowsNextQueued(t *testing.T) {
	for _, reason := range []string{"maxItems", "maxitems", "completed", "cursor_match", ""} {
		st := ApplyStop(AccountState{AccountID: 1, Status: StatusRunning}, reason, testNow, DefaultConfig())
		if st.Status != StatusReady {
			t.Errorf("clean stop %q → ready, got %s", reason, st.Status)
		}
	}
	// After the running account finished, the machine has a free slot for the queue.
	m := MachineState{Config: DefaultConfig(), Accounts: []AccountState{
		{AccountID: 1, Status: StatusReady}, // just finished
		{AccountID: 2, Status: StatusQueued, QueuedAt: testNow},
	}}
	got, ok := NextAccountToRun(m, testNow)
	if !ok {
		t.Fatal("with a free slot, a queued account must be selectable")
	}
	// Deterministic: account 1 is ready with a zero QueuedAt, which sorts before
	// account 2's real QueuedAt, so FIFO selection must pick account 1.
	if got != 1 {
		t.Errorf("account 1 (ready, zero QueuedAt) sorts earliest → must be selected, got %d", got)
	}
}

// Stall/exhaustion exits map to stalled_no_progress (distinct from ready), and
// are NOT human-required (they may run again once eligible).
func TestStalledReasonsMapToStalled(t *testing.T) {
	for _, reason := range []string{
		ReasonNoProgress, ReasonNoNewItemsAfterScroll, ReasonDuplicateHeavy,
		ReasonScrollNotMoving, ReasonPassExhausted,
	} {
		st := ApplyStop(AccountState{AccountID: 1, Status: StatusRunning}, reason, testNow, DefaultConfig())
		if st.Status != StatusStalledNoProgress {
			t.Errorf("%s → stalled_no_progress, got %s", reason, st.Status)
		}
		if IsHumanRequired(st.Status) {
			t.Errorf("%s must NOT be human-required", reason)
		}
		if st.LastSafeStopReason != reason {
			t.Errorf("%s must record last stop reason, got %q", reason, st.LastSafeStopReason)
		}
	}
}

// stalled_no_progress eligibility: with the default 0 cooldown it is immediately
// eligible (no artificial backoff — the recurring interval + FIFO gate re-runs);
// with a positive cooldown it is skipped until cooldown_until, exactly like
// cooling_down.
func TestStalledEligibilitySemantics(t *testing.T) {
	// Default 0 cooldown → CooldownUntil zero → eligible now.
	zero := ApplyStop(AccountState{AccountID: 1, Status: StatusRunning}, ReasonNoProgress, testNow, DefaultConfig())
	if !zero.CooldownUntil.IsZero() {
		t.Errorf("default 0 stalled cooldown → zero CooldownUntil, got %v", zero.CooldownUntil)
	}
	if !IsEligible(zero, testNow) {
		t.Error("stalled with 0 cooldown must be immediately eligible")
	}
	// Positive cooldown → skipped until it elapses.
	cfg := Config{MaxActiveCrawlsPerMachine: 1, StalledNoProgressCooldown: 4 * time.Minute}
	backoff := ApplyStop(AccountState{AccountID: 1, Status: StatusRunning}, ReasonDuplicateHeavy, testNow, cfg)
	if backoff.Status != StatusStalledNoProgress {
		t.Fatalf("expected stalled_no_progress, got %s", backoff.Status)
	}
	if IsEligible(backoff, testNow) {
		t.Error("stalled with a positive cooldown must be skipped before cooldown_until")
	}
	if !IsEligible(backoff, testNow.Add(4*time.Minute)) {
		t.Error("stalled must become eligible at cooldown_until")
	}
}

// Clean-run pacing cooldown is applied only when configured (never raises risk).
func TestCleanRunCooldownOptIn(t *testing.T) {
	cfg := Config{MaxActiveCrawlsPerMachine: 1, CleanRunCooldown: 3 * time.Minute}
	st := ApplyStop(AccountState{AccountID: 1, Status: StatusRunning}, "completed", testNow, cfg)
	if st.Status != StatusCoolingDown {
		t.Fatalf("with a pacing gap, a clean finish → cooling_down, got %s", st.Status)
	}
	if !st.CooldownUntil.Equal(testNow.Add(3 * time.Minute)) {
		t.Errorf("cooldown_until = now+gap, got %v", st.CooldownUntil)
	}
	if IsEligible(st, testNow) || !IsEligible(st, testNow.Add(3*time.Minute)) {
		t.Error("cooling_down must expire exactly at cooldown_until")
	}
}
