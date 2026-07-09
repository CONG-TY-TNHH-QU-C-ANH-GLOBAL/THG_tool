package accountsafety

import (
	"sync"
	"time"
)

// Coordinator is the in-memory runtime holder around the pure policy. It caps
// concurrent crawls per machine (default 1) and tracks per-account safety state.
//
// IN-MEMORY ONLY (PR-C4): state resets on process restart — acceptable because
// the recurring-intent interval remains the durable retry cadence, and a fresh
// process simply re-derives an empty budget. Durable state is a later migration
// PR. A running account is auto-released after runningStaleAfter so a lost
// result-feedback or a crash mid-crawl can NEVER wedge the machine budget.
//
// Concurrency: every exported method takes the mutex; the pure policy it calls
// is side-effect free, so the lock scope stays small.
type Coordinator struct {
	mu                sync.Mutex
	cfg               Config
	runningStaleAfter time.Duration
	states            map[int64]AccountState
	runningSince      map[int64]time.Time
}

// NewCoordinator builds a coordinator. runningStaleAfter MUST exceed the longest
// possible crawl (extension worst case ~10 min) so a live crawl is never freed.
// A non-positive budget is clamped to the binding safe default of 1.
func NewCoordinator(cfg Config, runningStaleAfter time.Duration) *Coordinator {
	if cfg.MaxActiveCrawlsPerMachine <= 0 {
		cfg.MaxActiveCrawlsPerMachine = 1
	}
	return &Coordinator{
		cfg:               cfg,
		runningStaleAfter: runningStaleAfter,
		states:            map[int64]AccountState{},
		runningSince:      map[int64]time.Time{},
	}
}

// releaseStale frees running accounts whose crawl has clearly ended (no feedback
// within runningStaleAfter). Caller MUST hold mu.
func (c *Coordinator) releaseStale(now time.Time) {
	if c.runningStaleAfter <= 0 {
		return
	}
	for id, since := range c.runningSince {
		if now.Sub(since) >= c.runningStaleAfter {
			delete(c.runningSince, id)
			c.states[id] = AccountState{AccountID: id, Status: StatusReady}
		}
	}
}

// machineLocked builds a MachineState value from current state. Caller holds mu.
func (c *Coordinator) machineLocked() MachineState {
	accts := make([]AccountState, 0, len(c.states))
	for _, st := range c.states {
		accts = append(accts, st)
	}
	return MachineState{Config: c.cfg, Accounts: accts}
}

// FreeSlots is how many new crawls may start now (budget minus running), after
// releasing stale runs. Drives the scheduler's claim limit so extra due accounts
// stay unclaimed (still due) for the next tick — a clean cross-tick queue that
// needs no change to ClaimDueIntents semantics.
func (c *Coordinator) FreeSlots(now time.Time) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releaseStale(now)
	free := c.cfg.MaxActiveCrawlsPerMachine - c.machineLocked().activeCount()
	if free < 0 {
		return 0
	}
	return free
}

// MarkRunning records that a crawl was dispatched for accountID (called at submit).
func (c *Coordinator) MarkRunning(accountID int64, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states[accountID] = AccountState{AccountID: accountID, Status: StatusRunning}
	c.runningSince[accountID] = now
}

// CanStart reports whether accountID may start now: budget free AND eligible AND
// not parked in a human-required state.
func (c *Coordinator) CanStart(accountID int64, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releaseStale(now)
	return CanStartAccount(accountID, c.machineLocked(), now)
}

// NextToRun returns the FIFO-earliest eligible account, honoring the budget.
func (c *Coordinator) NextToRun(now time.Time) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releaseStale(now)
	return NextAccountToRun(c.machineLocked(), now)
}

// Finish applies a crawl result to accountID (PR-C4B result-feedback): risk exits
// park the account (human-required, no auto-clear); stalled → stalled_no_progress;
// clean → ready. Always clears the running timer, freeing the machine slot
// immediately — the stale timeout is never needed for a reported result. Returns
// the resulting status for the caller's log line.
func (c *Coordinator) Finish(accountID int64, exitReason string, now time.Time) Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.runningSince, accountID)
	prev := c.states[accountID]
	prev.AccountID = accountID
	next := ApplyStop(prev, exitReason, now, c.cfg)
	c.states[accountID] = next
	return next.Status
}

// IsAccountEligible reports whether accountID may be dispatched now, ignoring the
// machine budget (the scheduler checks FreeSlots separately). Accounts the
// coordinator has never seen are eligible — a fresh process defaults to ready;
// only a recorded parked/cooling state blocks dispatch.
func (c *Coordinator) IsAccountEligible(accountID int64, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releaseStale(now)
	st, ok := c.states[accountID]
	if !ok {
		return true
	}
	return IsEligible(st, now)
}

// Resolve is the operator/verifier path out of a human-required-class state
// (wired later to CheckpointManager). Never fires on a timer.
func (c *Coordinator) Resolve(accountID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if st, ok := c.states[accountID]; ok {
		c.states[accountID] = ResolveHumanRequired(st)
	}
}

// Snapshot is a lock-safe view for logs/tests: active count + per-account status.
type Snapshot struct {
	Active   int
	Accounts map[int64]Status
}

func (c *Coordinator) Snapshot(now time.Time) Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releaseStale(now)
	s := Snapshot{Accounts: make(map[int64]Status, len(c.states))}
	for id, st := range c.states {
		s.Accounts[id] = st.Status
		if st.Status == StatusRunning {
			s.Active++
		}
	}
	return s
}
