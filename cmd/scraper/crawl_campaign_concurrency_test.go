package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thg/scraper/internal/services/facebook/crawlcampaign"
	"github.com/thg/scraper/internal/session/accountsafety"
)

// Local stubs for the crawlcampaign ports. Only the safety gate is the real
// shared coordinator (via campaignSafetyGate) — that is the collaborator under
// test; everything else is a deterministic stand-in.

type stubPools struct{ pools []crawlcampaign.OrgPool }

func (s stubPools) ActiveCampaignPools(context.Context) ([]crawlcampaign.OrgPool, error) {
	return s.pools, nil
}

type stubEnqueuer struct{}

func (stubEnqueuer) EnqueueDueRuns(context.Context, int64, time.Time) error { return nil }

type stubReady struct{}

func (stubReady) Ready(context.Context, int64, int64) bool { return true }

type stubClaimer struct {
	ok    bool
	claim crawlcampaign.PooledClaim
}

func (s stubClaimer) ClaimNextRun(_ context.Context, orgID, accountID int64, _ time.Time) (crawlcampaign.PooledClaim, bool, error) {
	if !s.ok {
		return crawlcampaign.PooledClaim{}, false, nil
	}
	c := s.claim
	c.Fence.OrgID, c.AccountID = orgID, accountID
	return c, true, nil
}

type stubRecoverer struct{}

func (stubRecoverer) RecoverDispatchFailure(context.Context, crawlcampaign.RunFence, int64, time.Time) error {
	return nil
}

type countingDispatcher struct{ n int32 }

func (d *countingDispatcher) Dispatch(context.Context, crawlcampaign.PooledClaim) error {
	atomic.AddInt32(&d.n, 1)
	return nil
}

// barrierGate wraps the real safety gate so both schedulers arrive at TryReserve
// before either calls through to the coordinator — forcing genuine contention on
// the one machine slot without any sleep.
type barrierGate struct {
	inner  crawlcampaign.AccountSafetyGate
	arrive func()
}

func (b barrierGate) Eligible(a int64, now time.Time) bool { return b.inner.Eligible(a, now) }
func (b barrierGate) TryReserve(a int64, now time.Time) bool {
	b.arrive()
	return b.inner.TryReserve(a, now)
}
func (b barrierGate) Release(a int64, reason string, now time.Time) { b.inner.Release(a, reason, now) }

func newStubOrchestrator(pool crawlcampaign.OrgPool, run crawlcampaign.PooledClaim, safety crawlcampaign.AccountSafetyGate, disp crawlcampaign.CrawlCommandDispatcher) *crawlcampaign.Orchestrator {
	return crawlcampaign.New(crawlcampaign.Deps{
		Pools:      stubPools{[]crawlcampaign.OrgPool{pool}},
		Enqueuer:   stubEnqueuer{},
		Claimer:    stubClaimer{ok: true, claim: run},
		Recoverer:  stubRecoverer{},
		Safety:     safety,
		Readiness:  stubReady{},
		Dispatcher: disp,
	})
}

// Two schedulers on the SAME coordinator both reach a claimable run at the same
// instant. The atomic TryReserve must let exactly one win the single machine
// slot, so exactly one dispatches and the other does not — no double-dispatch,
// no budget doubling.
func TestTwoSchedulersShareOneMachineSlotAtomically(t *testing.T) {
	coord := accountsafety.NewCoordinator(accountsafety.DefaultConfig(), 15*time.Minute)
	base := campaignSafetyGate{coord}
	now := time.Now().UTC()

	var ready sync.WaitGroup
	ready.Add(2)
	arrive := func() { ready.Done(); ready.Wait() } // two-party barrier

	disp1 := &countingDispatcher{}
	disp2 := &countingDispatcher{}
	orch1 := newStubOrchestrator(
		crawlcampaign.OrgPool{OrgID: 1, AccountIDs: []int64{101}},
		crawlcampaign.PooledClaim{Fence: crawlcampaign.RunFence{RunID: 9, Attempt: 1}},
		barrierGate{base, arrive}, disp1)
	orch2 := newStubOrchestrator(
		crawlcampaign.OrgPool{OrgID: 2, AccountIDs: []int64{202}},
		crawlcampaign.PooledClaim{Fence: crawlcampaign.RunFence{RunID: 10, Attempt: 1}},
		barrierGate{base, arrive}, disp2)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = orch1.RunOnce(context.Background(), now) }()
	go func() { defer wg.Done(); _ = orch2.RunOnce(context.Background(), now) }()
	wg.Wait()

	total := atomic.LoadInt32(&disp1.n) + atomic.LoadInt32(&disp2.n)
	if total != 1 {
		t.Fatalf("exactly one scheduler may dispatch under contention for one slot, got %d (disp1=%d disp2=%d)",
			total, disp1.n, disp2.n)
	}
	if got := coord.FreeSlots(now); got != 0 {
		t.Fatalf("the winning dispatch must hold the one machine slot, free=%d want 0", got)
	}
}

// A reservation the orchestrator backs out of (the claim missed) must return the
// slot to the real coordinator — no leak.
func TestReservationReleasedOnClaimMiss(t *testing.T) {
	coord := accountsafety.NewCoordinator(accountsafety.DefaultConfig(), 15*time.Minute)
	now := time.Now().UTC()
	orch := crawlcampaign.New(crawlcampaign.Deps{
		Pools:      stubPools{[]crawlcampaign.OrgPool{{OrgID: 1, AccountIDs: []int64{101}}}},
		Enqueuer:   stubEnqueuer{},
		Claimer:    stubClaimer{ok: false}, // reserve succeeds, then the claim misses
		Recoverer:  stubRecoverer{},
		Safety:     campaignSafetyGate{coord},
		Readiness:  stubReady{},
		Dispatcher: &countingDispatcher{},
	})
	if err := orch.RunOnce(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if got := coord.FreeSlots(now); got != 1 {
		t.Fatalf("claim miss must release the reserved slot, free=%d want 1 (leak)", got)
	}
}
