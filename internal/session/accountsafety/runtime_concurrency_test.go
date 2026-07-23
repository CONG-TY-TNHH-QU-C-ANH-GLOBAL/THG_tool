package accountsafety

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TryMarkRunning must be atomic: with the default machine budget of 1, many
// goroutines racing to claim the one slot must yield exactly one winner. This
// pins the regression a FreeSlots-then-MarkRunning sequence would reintroduce,
// where two callers observe the same free slot and both mark running.
func TestTryMarkRunningAtomicUnderContention(t *testing.T) {
	c := newCoord() // budget = 1

	const goroutines = 64
	var start sync.WaitGroup
	start.Add(goroutines)
	var done sync.WaitGroup
	done.Add(goroutines)
	var wins int32

	for i := range goroutines {
		go func(id int) {
			defer done.Done()
			start.Done()
			start.Wait() // release every goroutine at once — force real contention
			if c.TryMarkRunning(int64(1000+id), testNow) {
				atomic.AddInt32(&wins, 1)
			}
		}(i)
	}
	done.Wait()

	if wins != 1 {
		t.Fatalf("budget=1: exactly one TryMarkRunning may win under contention, got %d", wins)
	}
	if got := c.FreeSlots(testNow); got != 0 {
		t.Fatalf("one slot must be held by the sole winner, free=%d want 0", got)
	}
}

// TryMarkRunning validates eligibility inside the same lock as the budget check,
// so a parked (human-required) account cannot reserve even while the slot is
// free, and a running account cannot reserve a second slot for itself. This pins
// the eligibility TOCTOU an IsAccountEligible-then-mark sequence would leave open.
func TestTryMarkRunningRejectsIneligibleAccount(t *testing.T) {
	c := newCoord() // budget 1

	// Park account 7 (checkpoint → human-required). It is not "running", so it
	// does not consume the slot, but it must not be reservable.
	c.MarkRunning(7, testNow)
	c.Finish(7, ReasonCheckpointSuspected, testNow)
	if c.TryMarkRunning(7, testNow) {
		t.Fatal("a parked (checkpoint) account must not reserve a machine slot")
	}
	if got := c.FreeSlots(testNow); got != 1 {
		t.Fatalf("a parked account must not hold the slot, free=%d want 1", got)
	}

	// An eligible (unseen) account may take the free slot.
	if !c.TryMarkRunning(8, testNow) {
		t.Fatal("an eligible account must reserve the free slot")
	}
	// The now-running account 8 cannot reserve a second slot for itself.
	if c.TryMarkRunning(8, testNow) {
		t.Fatal("an already-running account must not reserve a second slot")
	}
}
