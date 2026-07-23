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

// The ACTUAL legacy recurring-intent acquisition path (crawlIntentDispatcher.
// dispatch, which now reserves atomically via TryMarkRunning before submit) and
// the ACTUAL Fresh Lead campaign acquisition path (the orchestrator through
// campaignSafetyGate) compete for the one machine slot on the SAME coordinator.
// A deterministic two-party barrier releases both at their reservation point (no
// sleeps), so they genuinely contend. The atomic acquisition must let exactly one
// win: exactly one dispatches, active count stays 1, the loser neither claims nor
// dispatches, and the loser leaks no slot.
//
// Helpers reused from crawl_scheduler_safety_test.go (seedRoutableSession,
// seedDueCrawlIntent), direct_post_intake_test.go (newIntakeEnv, countImportJobs)
// and crawl_campaign_concurrency_test.go (barrierGate, countingDispatcher,
// newStubOrchestrator). This test is additive — it does not replace the
// campaign-vs-campaign test.
func TestLegacyAndCampaignSchedulersShareOneMachineSlot(t *testing.T) {
	ctx := context.Background()
	db, js := newIntakeEnv(t)
	coord := accountsafety.NewCoordinator(accountsafety.DefaultConfig(), 15*time.Minute) // budget = 1
	now := time.Now().UTC()

	// Legacy side: a due recurring intent on account 101 with a routable session so
	// dispatch reaches submit. dispatch() is the real legacy acquisition method.
	seedRoutableSession(t, db, 1, 101)
	intent := seedDueCrawlIntent(t, db, 1, 101, "https://facebook.com/groups/legacy", now.Add(-time.Minute))
	legacy := crawlIntentDispatcher{db: db, jobStore: js, coord: coord}

	// Campaign side: the real orchestrator on the SAME coordinator, its reservation
	// barriered right before TryReserve.
	campDisp := &countingDispatcher{}

	var ready sync.WaitGroup
	ready.Add(2)
	arrive := func() { ready.Done(); ready.Wait() } // two-party barrier

	campaign := newStubOrchestrator(
		crawlcampaign.OrgPool{OrgID: 2, AccountIDs: []int64{202}},
		crawlcampaign.PooledClaim{Fence: crawlcampaign.RunFence{RunID: 9, Attempt: 1}},
		barrierGate{campaignSafetyGate{coord}, arrive}, campDisp)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); arrive(); legacy.dispatch(ctx, *intent, now) }()
	go func() { defer wg.Done(); _ = campaign.RunOnce(ctx, now) }()
	wg.Wait()

	// Exactly one path holds the single machine slot.
	if active := coord.Snapshot(now).Active; active != 1 {
		t.Fatalf("active machine crawls = %d, want exactly 1 (budget of 1 must hold across schedulers)", active)
	}
	// A legacy job exists iff the legacy path reserved then submitted.
	legacyDispatched := countImportJobs(t, js) == 1
	campaignDispatched := atomic.LoadInt32(&campDisp.n) == 1
	if legacyDispatched == campaignDispatched {
		t.Fatalf("exactly one scheduler may dispatch under contention: legacy=%v campaign=%v", legacyDispatched, campaignDispatched)
	}
	// active == 1 already proves the loser leaked no slot: a leak would read as 2.
}
