package runtime

import (
	"fmt"
	"sync"
	"time"
)

// Budget enforces hard cost/time limits on a single job run.
// Every FetchBatch call must call RecordBatch() then CheckOrAbort().
// Exceeding either limit aborts immediately — no further retries, no LLM calls.
type Budget struct {
	mu         sync.Mutex
	startedAt  time.Time
	maxRuntime time.Duration
	maxBatches int
	batches    int
}

// DefaultBudget is the production safety ceiling per job.
// 60s wall-clock + 30 batch calls (each may invoke CDP + LLM).
var DefaultBudget = BudgetConfig{
	MaxRuntime: 60 * time.Second,
	MaxBatches: 30,
}

type BudgetConfig struct {
	MaxRuntime time.Duration
	MaxBatches int
}

func NewBudget(cfg BudgetConfig) *Budget {
	return &Budget{
		startedAt:  time.Now(),
		maxRuntime: cfg.MaxRuntime,
		maxBatches: cfg.MaxBatches,
	}
}

// RecordBatch increments the batch counter. Call once per FetchBatch invocation.
func (b *Budget) RecordBatch() {
	b.mu.Lock()
	b.batches++
	b.mu.Unlock()
}

// CheckOrAbort returns ErrBudgetExceeded when either limit is breached.
// Always call this after RecordBatch() — before issuing any network/LLM call.
func (b *Budget) CheckOrAbort() error {
	b.mu.Lock()
	elapsed := time.Since(b.startedAt)
	batches := b.batches
	b.mu.Unlock()

	if elapsed > b.maxRuntime {
		return CDPError{
			Code:    ErrBudgetExceeded,
			Message: fmt.Sprintf("ABORT: runtime %.0fs exceeded limit %.0fs", elapsed.Seconds(), b.maxRuntime.Seconds()),
		}
	}
	if batches > b.maxBatches {
		return CDPError{
			Code:    ErrBudgetExceeded,
			Message: fmt.Sprintf("ABORT: batch calls %d exceeded limit %d", batches, b.maxBatches),
		}
	}
	return nil
}

// Elapsed returns time spent since the budget was created.
func (b *Budget) Elapsed() time.Duration {
	return time.Since(b.startedAt)
}
