package accountsafety

import (
	"testing"
	"time"
)

// PR-C4 result-feedback contract: Finish frees the machine slot at the SAME
// instant the result arrives — the stale timeout (15m away) is a crash fallback,
// never part of normal completion. A 90-second crawl must not hold the slot.
func TestCoordinatorFinishFreesSlotWithoutStaleTimeout(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	finishAt := testNow.Add(90 * time.Second)
	if got := c.Finish(1, "completed", finishAt); got != StatusReady {
		t.Errorf("Finish(completed) status = %s, want ready", got)
	}
	if got := c.FreeSlots(finishAt); got != 1 {
		t.Errorf("free slots right after Finish = %d, want 1 (must not wait for the stale timeout)", got)
	}
}

// Every stalled exit_reason frees the slot, records stalled_no_progress, and
// (default 0 cooldown) leaves the account dispatchable.
func TestCoordinatorStalledReasonsFreeSlot(t *testing.T) {
	reasons := []string{ReasonNoProgress, ReasonNoNewItemsAfterScroll,
		ReasonDuplicateHeavy, ReasonScrollNotMoving, ReasonPassExhausted}
	for _, reason := range reasons {
		c := newCoord()
		c.MarkRunning(1, testNow)
		if got := c.Finish(1, reason, testNow); got != StatusStalledNoProgress {
			t.Errorf("Finish(%s) status = %s, want stalled_no_progress", reason, got)
		}
		if c.FreeSlots(testNow) != 1 {
			t.Errorf("Finish(%s) must free the machine slot immediately", reason)
		}
		if !c.IsAccountEligible(1, testNow) {
			t.Errorf("stalled account (default 0 cooldown) must stay dispatchable after %s", reason)
		}
	}
}

// Every risk exit_reason frees the slot for OTHER accounts but parks this one:
// no amount of elapsed time makes it eligible — only the operator Resolve path.
func TestCoordinatorRiskReasonsParkUntilResolve(t *testing.T) {
	parked := map[string]Status{
		ReasonCheckpointSuspected: StatusCheckpointRequired,
		ReasonLoginRequired:       StatusLoginRequired,
		ReasonRiskBlocked:         StatusRiskBlocked,
	}
	for reason, want := range parked {
		c := newCoord()
		c.MarkRunning(1, testNow)
		if got := c.Finish(1, reason, testNow); got != want {
			t.Errorf("Finish(%s) status = %s, want %s", reason, got, want)
		}
		if c.FreeSlots(testNow) != 1 {
			t.Errorf("Finish(%s) must free the machine slot for other accounts", reason)
		}
		if c.IsAccountEligible(1, testNow.Add(1000*time.Hour)) {
			t.Errorf("%s must keep the account parked regardless of elapsed time", reason)
		}
		c.Resolve(1)
		if !c.IsAccountEligible(1, testNow) {
			t.Errorf("operator Resolve must make the account dispatchable again after %s", reason)
		}
	}
}

// Scheduler eligibility gate defaults: an account the coordinator never saw is
// eligible (fresh process = ready); a running account is not dispatched again.
func TestCoordinatorIsAccountEligibleDefaults(t *testing.T) {
	c := newCoord()
	if !c.IsAccountEligible(42, testNow) {
		t.Error("an unknown account must be eligible by default")
	}
	c.MarkRunning(42, testNow)
	if c.IsAccountEligible(42, testNow) {
		t.Error("a running account must not be dispatched again")
	}
}

// An empty exit_reason follows the clean-stop policy default: ready, slot freed.
func TestCoordinatorEmptyExitReasonDefaultsClean(t *testing.T) {
	c := newCoord()
	c.MarkRunning(1, testNow)
	if got := c.Finish(1, "", testNow); got != StatusReady {
		t.Errorf("Finish(empty) status = %s, want ready (policy default)", got)
	}
	if c.FreeSlots(testNow) != 1 {
		t.Error("empty exit_reason must free the slot")
	}
}
