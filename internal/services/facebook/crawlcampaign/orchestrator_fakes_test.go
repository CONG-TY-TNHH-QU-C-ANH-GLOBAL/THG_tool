package crawlcampaign

import (
	"context"
	"testing"
	"time"
)

// The orchestrator is pure over its ports, so these fakes exercise the full
// selection/dispatch/recover ordering and the machine-budget cap without a
// database. The real Postgres mechanics are pinned by internal/store/crawlrun.

var fixedNow = time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

type fakePools struct {
	pools []OrgPool
	err   error
}

func (f *fakePools) ActiveCampaignPools(context.Context) ([]OrgPool, error) {
	return f.pools, f.err
}

type fakeEnqueuer struct {
	orgs   []int64
	errFor map[int64]error
}

func (f *fakeEnqueuer) EnqueueDueRuns(_ context.Context, orgID int64, _ time.Time) error {
	f.orgs = append(f.orgs, orgID)
	return f.errFor[orgID]
}

type fakeClaimer struct {
	// claimable account ids → the run they claim (one-shot each, so a re-claim
	// after dispatch returns ok=false like a real drained queue).
	claimable map[int64]PooledClaim
	err       error
	calls     []int64
}

func (f *fakeClaimer) ClaimNextRun(_ context.Context, orgID, accountID int64, _ time.Time) (PooledClaim, bool, error) {
	f.calls = append(f.calls, accountID)
	if f.err != nil {
		return PooledClaim{}, false, f.err
	}
	claim, ok := f.claimable[accountID]
	if !ok {
		return PooledClaim{}, false, nil
	}
	delete(f.claimable, accountID)
	claim.Fence.OrgID, claim.AccountID = orgID, accountID
	return claim, true, nil
}

type fakeRecoverer struct {
	calls []RunFence
	err   error
}

func (f *fakeRecoverer) RecoverDispatchFailure(_ context.Context, fence RunFence, _ int64, _ time.Time) error {
	f.calls = append(f.calls, fence)
	return f.err
}

type fakeSafety struct {
	free       int
	ineligible map[int64]bool
	reserved   []int64 // accounts whose TryReserve returned true
	released   []int64
}

func (f *fakeSafety) Eligible(accountID int64, _ time.Time) bool { return !f.ineligible[accountID] }

// TryReserve models the coordinator's atomic budget check-and-mark: it succeeds
// only while a slot is free, decrementing it in the same call.
func (f *fakeSafety) TryReserve(accountID int64, _ time.Time) bool {
	if f.free <= 0 {
		return false
	}
	f.free--
	f.reserved = append(f.reserved, accountID)
	return true
}
func (f *fakeSafety) Release(accountID int64, _ string, _ time.Time) {
	f.free++
	f.released = append(f.released, accountID)
}

type fakeReadiness struct{ notReady map[int64]bool }

func (f *fakeReadiness) Ready(_ context.Context, _, accountID int64) bool {
	return !f.notReady[accountID]
}

type fakeDispatcher struct {
	failFor  map[int64]error
	dispatch []int64
}

func (f *fakeDispatcher) Dispatch(_ context.Context, claim PooledClaim) error {
	f.dispatch = append(f.dispatch, claim.AccountID)
	return f.failFor[claim.AccountID]
}

// harness wires one orchestrator over fresh fakes with sane defaults: budget=1,
// everyone eligible+ready, account 101 claimable, dispatch succeeds.
type harness struct {
	pools *fakePools
	enq   *fakeEnqueuer
	claim *fakeClaimer
	rec   *fakeRecoverer
	safe  *fakeSafety
	ready *fakeReadiness
	disp  *fakeDispatcher
	orch  *Orchestrator
}

func newHarness() *harness {
	h := &harness{
		pools: &fakePools{pools: []OrgPool{{OrgID: 1, AccountIDs: []int64{101}}}},
		enq:   &fakeEnqueuer{errFor: map[int64]error{}},
		claim: &fakeClaimer{claimable: map[int64]PooledClaim{101: {Fence: RunFence{RunID: 9, Attempt: 1}, CampaignID: 5, SourceID: 7}}},
		rec:   &fakeRecoverer{},
		safe:  &fakeSafety{free: 1, ineligible: map[int64]bool{}},
		ready: &fakeReadiness{notReady: map[int64]bool{}},
		disp:  &fakeDispatcher{failFor: map[int64]error{}},
	}
	h.orch = New(Deps{
		Pools: h.pools, Enqueuer: h.enq, Claimer: h.claim, Recoverer: h.rec,
		Safety: h.safe, Readiness: h.ready, Dispatcher: h.disp,
	})
	return h
}

func (h *harness) run(t *testing.T) {
	t.Helper()
	if err := h.orch.RunOnce(context.Background(), fixedNow); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
}
