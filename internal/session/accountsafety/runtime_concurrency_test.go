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
