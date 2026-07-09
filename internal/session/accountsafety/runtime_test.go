package accountsafety

import (
	"testing"
	"time"
)

const staleAfter = 15 * time.Minute

func newCoord() *Coordinator { return NewCoordinator(DefaultConfig(), staleAfter) }

// #1 default budget admits only one active account.
func TestCoordinatorAdmitsOne(t *testing.T) {
	c := newCoord()
	if got := c.FreeSlots(testNow); got != 1 {
		t.Fatalf("fresh coordinator free slots = %d, want 1", got)
	}
	c.MarkRunning(1, testNow)
	if got := c.FreeSlots(testNow); got != 0 {
		t.Errorf("with one running, free slots = %d, want 0", got)
	}
	if c.Snapshot(testNow).Active != 1 {
		t.Errorf("active count must be 1")
	}
}

// #2 a second account is not admitted while one runs: the machine BUDGET blocks
// it (FreeSlots=0 → the scheduler claims nothing), not its per-account state —
// an idle second account stays state-eligible for the next free slot.
func TestCoordinatorSecondQueued(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	if c.FreeSlots(testNow) != 0 {
		t.Error("no free slots while one account runs")
	}
	if !c.IsAccountEligible(2, testNow) {
		t.Error("account 2 stays state-eligible; only the budget blocks it")
	}
}

// #3 a completed account frees the budget, and an account the coordinator never
// saw is eligible by default (first-time crawl accounts must not be blocked).
func TestCoordinatorCompletedFreesBudget(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	c.Finish(1, "completed", testNow)
	if got := c.FreeSlots(testNow); got != 1 {
		t.Errorf("after completion free slots = %d, want 1", got)
	}
	if !c.IsAccountEligible(2, testNow) {
		t.Error("an unseen account must be eligible for the freed slot")
	}
}

// #4 checkpoint_suspected parks the account; it does not auto-retry on later ticks.
func TestCoordinatorCheckpointParks(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	c.Finish(1, ReasonCheckpointSuspected, testNow)
	if c.IsAccountEligible(1, testNow.Add(1000*time.Hour)) {
		t.Error("checkpoint_required must NOT become eligible by time")
	}
	if got := c.Snapshot(testNow).Accounts[1]; got != StatusCheckpointRequired {
		t.Errorf("status = %s, want checkpoint_required", got)
	}
	// Operator path is the only exit.
	c.Resolve(1)
	if !c.IsAccountEligible(1, testNow) {
		t.Error("after operator resolve the account must be eligible")
	}
}

// #5 login_required parks the account and does not auto-retry.
func TestCoordinatorLoginParks(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	c.Finish(1, ReasonLoginRequired, testNow)
	if c.IsAccountEligible(1, testNow.Add(1000*time.Hour)) {
		t.Error("login_required must NOT become eligible by time")
	}
}

// #6 stalled_no_progress is not human-required and (default 0 cooldown) remains runnable.
func TestCoordinatorStalledNotHumanRequired(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	c.Finish(1, ReasonNoProgress, testNow)
	st := c.Snapshot(testNow).Accounts[1]
	if st != StatusStalledNoProgress {
		t.Fatalf("status = %s, want stalled_no_progress", st)
	}
	if IsHumanRequired(st) {
		t.Error("stalled_no_progress must not be human-required")
	}
	if !c.IsAccountEligible(1, testNow) {
		t.Error("stalled_no_progress with default 0 cooldown must be eligible")
	}
}

// #8 process-restart semantics: a fresh coordinator starts with a full budget and
// no memory of prior runs (documented in-memory limitation; durable state = later PR).
func TestCoordinatorRestartSemantics(t *testing.T) {
	fresh := newCoord()
	if fresh.FreeSlots(testNow) != 1 {
		t.Error("a restarted (empty) coordinator must offer the full budget")
	}
	if len(fresh.Snapshot(testNow).Accounts) != 0 {
		t.Error("a restarted coordinator has no account memory")
	}
}

// Safety fallback: a running account with no result-feedback is auto-released
// after runningStaleAfter, so a lost result never wedges the machine budget.
func TestCoordinatorStaleRunningAutoFree(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	if c.FreeSlots(testNow.Add(staleAfter - time.Minute)) != 0 {
		t.Error("before the stale window the slot stays taken")
	}
	if c.FreeSlots(testNow.Add(staleAfter)) != 1 {
		t.Error("at/after the stale window the running slot auto-frees")
	}
}
