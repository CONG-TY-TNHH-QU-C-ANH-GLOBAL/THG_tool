package embedding

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Goal G3 (Jobs Runtime): the codebase has a HARD RULE against
// unbounded goroutines and fire-and-forget background loops. This
// file extends Worker with a bounded, graceful-shutdown harness
// that production should always use instead of raw `go w.Run(ctx)`.
//
// What makes a goroutine "unbounded" vs "bounded" for this rule:
//
//   UNBOUNDED (forbidden):
//     - Started with `go w.Run(ctx)` without tracking completion.
//     - Cannot be reliably stopped within a deadline.
//     - Leaks if the caller forgets to cancel ctx.
//
//   BOUNDED (acceptable):
//     - Started via Supervisor, which holds a WaitGroup and a
//       shutdown channel.
//     - Stop() waits up to MaxShutdown for the goroutine to exit;
//       if it doesn't, returns ErrShutdownTimeout so the operator
//       can decide (kill the binary vs wait longer).
//     - Tests can assert "no goroutine leaked" using runtime.NumGoroutine
//       before/after.

// ErrShutdownTimeout is returned by Supervisor.Stop when the worker
// did not exit within MaxShutdown. Surfaced to the operator so they
// can decide whether to force-kill the process.
var ErrShutdownTimeout = errors.New("embedding supervisor: worker did not exit within MaxShutdown")

// Supervisor is a bounded-lifetime wrapper around Worker. Construct
// via [NewSupervisor], then call Start to launch and Stop to wait
// for graceful exit.
//
// One Supervisor manages exactly ONE worker. Callers that want
// multiple worker replicas hold a slice of Supervisors and Stop
// each in turn (or in parallel — Stop is concurrency-safe).
type Supervisor struct {
	worker *Worker

	// MaxShutdown caps how long Stop will wait before declaring the
	// worker leaked. Default 30s. Tune up for embedders that hold
	// large in-flight batches.
	MaxShutdown time.Duration

	started bool
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	runErr  error
}

// NewSupervisor wraps a Worker with bounded-lifetime supervision.
// Sane defaults: 30s graceful shutdown ceiling. Callers may override
// MaxShutdown before calling Start.
func NewSupervisor(w *Worker) *Supervisor {
	if w == nil {
		panic("embedding.NewSupervisor: nil worker")
	}
	return &Supervisor{
		worker:      w,
		MaxShutdown: 30 * time.Second,
	}
}

// Start launches the worker goroutine. Idempotent — second call is
// a no-op. Returns immediately; the worker runs until Stop is
// called or the parent context is cancelled.
//
// parentCtx is the lifecycle ceiling. If parentCtx is cancelled
// (e.g. the binary is shutting down), the worker exits regardless
// of Stop being called. This is the "binary dies" path; Stop is the
// "we're done, drain gracefully" path.
func (s *Supervisor) Start(parentCtx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true
	s.done = make(chan struct{})

	ctx, cancel := context.WithCancel(parentCtx)
	s.cancel = cancel

	go func() {
		defer close(s.done)
		s.runErr = s.worker.Run(ctx)
	}()
}

// Stop signals the worker to exit and waits up to MaxShutdown for it
// to actually exit. Returns nil on clean shutdown, ErrShutdownTimeout
// if the worker is still running after the timeout.
//
// Stop is concurrency-safe AND idempotent — multiple callers can
// invoke it; only the first cancellation takes effect; subsequent
// calls observe the same outcome.
func (s *Supervisor) Stop() error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	cancel := s.cancel
	done := s.done
	maxWait := s.MaxShutdown
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	select {
	case <-done:
		// Clean exit. We deliberately discard runErr — context-cancel
		// returns ctx.Err() which is the expected outcome of Stop.
		return nil
	case <-time.After(maxWait):
		return ErrShutdownTimeout
	}
}

// IsRunning reports whether the worker goroutine is alive. Useful
// for /healthz and supervisor-of-supervisors patterns.
func (s *Supervisor) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.done == nil {
		return false
	}
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// RunErr returns the worker's exit error, available only after Stop
// (or parentCtx cancellation) has been observed. Mostly context.Canceled
// in production; non-context errors indicate the worker crashed.
func (s *Supervisor) RunErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runErr
}
