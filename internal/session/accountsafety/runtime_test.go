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

// #2 a second account is not admitted while one runs (scheduler would claim 0).
func TestCoordinatorSecondQueued(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	if c.CanStart(2, testNow) {
		t.Error("account 2 must not start while the machine budget is full")
	}
	if c.FreeSlots(testNow) != 0 {
		t.Error("no free slots while one account runs")
	}
}

// #3 a completed account frees the budget for the next account.
func TestCoordinatorCompletedFreesBudget(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	c.Finish(1, "completed", testNow)
	if got := c.FreeSlots(testNow); got != 1 {
		t.Errorf("after completion free slots = %d, want 1", got)
	}
	if c.CanStart(2, testNow) {
		t.Error("account 2 unknown to coordinator is not startable until registered")
	}
}

// #4 checkpoint_suspected parks the account; it does not auto-retry on later ticks.
func TestCoordinatorCheckpointParks(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	c.Finish(1, ReasonCheckpointSuspected, testNow)
	if c.CanStart(1, testNow.Add(1000*time.Hour)) {
		t.Error("checkpoint_required must NOT become startable by time")
	}
	if got := c.Snapshot(testNow).Accounts[1]; got != StatusCheckpointRequired {
		t.Errorf("status = %s, want checkpoint_required", got)
	}
	// Operator path is the only exit.
	c.Resolve(1)
	if !c.CanStart(1, testNow) {
		t.Error("after operator resolve the account must be startable")
	}
}

// #5 login_required parks the account and does not auto-retry.
func TestCoordinatorLoginParks(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	c.Finish(1, ReasonLoginRequired, testNow)
	if c.CanStart(1, testNow.Add(1000*time.Hour)) {
		t.Error("login_required must NOT become startable by time")
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
	if !c.CanStart(1, testNow) {
		t.Error("stalled_no_progress with default 0 cooldown must be startable")
	}
}

// #7 FIFO ordering: the earliest-queued eligible account is chosen.
func TestCoordinatorFIFO(t *testing.T) {
	c := NewCoordinator(Config{MaxActiveCrawlsPerMachine: 1}, staleAfter)
	c.states[10] = AccountState{AccountID: 10, Status: StatusQueued, QueuedAt: testNow.Add(2 * time.Minute)}
	c.states[11] = AccountState{AccountID: 11, Status: StatusQueued, QueuedAt: testNow.Add(1 * time.Minute)}
	got, ok := c.NextToRun(testNow)
	if !ok || got != 11 {
		t.Errorf("FIFO must pick earliest queued (11), got %d ok=%v", got, ok)
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
