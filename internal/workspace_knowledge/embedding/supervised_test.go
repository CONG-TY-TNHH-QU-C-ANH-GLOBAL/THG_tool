package embedding

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// G3 goal directive: bounded shutdown, no goroutine leak.
//
// The proof is observable: snapshot runtime.NumGoroutine() before
// Supervisor.Start, after Stop, and after a generous settle window.
// They must match within a small noise floor (test runner has its
// own goroutines that may shift by 1-2). The strict invariant we
// assert is "Stop returns within MaxShutdown" + "no leftover
// supervisor-owned goroutines after Stop".

func TestSupervisor_CleanShutdownWithinBudget(t *testing.T) {
	store := &recordingStore{}
	emb := newMockEmbedder()
	w := NewWorker(store, emb)
	w.PollInterval = 10 * time.Millisecond // tight loop for the test
	sup := NewSupervisor(w)
	sup.MaxShutdown = 200 * time.Millisecond

	before := runtime.NumGoroutine()
	sup.Start(context.Background())
	// Let it run a few ticks.
	time.Sleep(50 * time.Millisecond)
	if !sup.IsRunning() {
		t.Fatal("supervisor reports not running but Start was called")
	}

	start := time.Now()
	if err := sup.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	took := time.Since(start)
	if took > 200*time.Millisecond {
		t.Errorf("Stop exceeded MaxShutdown: %v", took)
	}
	if sup.IsRunning() {
		t.Error("worker still running after Stop")
	}

	// Goroutine leak detection — allow up to 2 stragglers from the
	// test runtime; anything more is a real leak.
	time.Sleep(30 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Errorf("goroutine leak: before=%d after=%d", before, after)
	}
}

// Stop called twice is idempotent and never panics or hangs.
func TestSupervisor_StopIsIdempotent(t *testing.T) {
	store := &recordingStore{}
	w := NewWorker(store, newMockEmbedder())
	w.PollInterval = 5 * time.Millisecond
	sup := NewSupervisor(w)
	sup.MaxShutdown = 100 * time.Millisecond

	sup.Start(context.Background())
	time.Sleep(20 * time.Millisecond)

	if err := sup.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	// Second call should be a no-op clean return.
	done := make(chan error, 1)
	go func() { done <- sup.Stop() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("second Stop returned non-nil: %v", err)
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("second Stop hung")
	}
}

// parentCtx cancellation triggers shutdown even WITHOUT Stop being
// called — proves the supervisor respects the lifecycle ceiling.
func TestSupervisor_ParentContextCancelStopsWorker(t *testing.T) {
	store := &recordingStore{}
	w := NewWorker(store, newMockEmbedder())
	w.PollInterval = 5 * time.Millisecond
	sup := NewSupervisor(w)
	sup.MaxShutdown = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	sup.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	cancel()

	// Stop must return cleanly because parent cancellation already
	// drove the worker to exit.
	if err := sup.Stop(); err != nil {
		t.Errorf("Stop after parent cancel: %v", err)
	}
	if sup.IsRunning() {
		t.Error("worker still running after parent ctx cancel")
	}
}

// Concurrent Stops from many goroutines: one should win, all should
// return cleanly. Proves the supervisor is safe under contention
// (an orchestrator may receive multiple shutdown signals).
func TestSupervisor_ConcurrentStops(t *testing.T) {
	store := &recordingStore{}
	w := NewWorker(store, newMockEmbedder())
	w.PollInterval = 5 * time.Millisecond
	sup := NewSupervisor(w)
	sup.MaxShutdown = 200 * time.Millisecond
	sup.Start(context.Background())
	time.Sleep(15 * time.Millisecond)

	const concurrency = 8
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- sup.Stop()
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil && err != ErrShutdownTimeout {
			t.Errorf("concurrent Stop: %v", err)
		}
	}
}

// Start twice is idempotent. Operator-deploy scripts sometimes call
// Start in a retry loop; we should not spawn duplicate workers.
func TestSupervisor_StartIsIdempotent(t *testing.T) {
	store := &recordingStore{}
	w := NewWorker(store, newMockEmbedder())
	w.PollInterval = 5 * time.Millisecond
	sup := NewSupervisor(w)
	defer sup.Stop()

	before := runtime.NumGoroutine()
	sup.Start(context.Background())
	sup.Start(context.Background())
	sup.Start(context.Background())
	time.Sleep(15 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+3 {
		t.Errorf("multiple Start calls spawned multiple goroutines: before=%d after=%d", before, after)
	}
}
