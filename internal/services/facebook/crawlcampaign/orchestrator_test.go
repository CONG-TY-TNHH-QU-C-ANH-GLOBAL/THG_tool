package crawlcampaign

import (
	"context"
	"errors"
	"testing"
)

// Fakes + harness live in orchestrator_fakes_test.go.

func TestHappyPathDispatchesOnceAndReserves(t *testing.T) {
	h := newHarness()
	h.run(t)
	if len(h.disp.dispatch) != 1 || h.disp.dispatch[0] != 101 {
		t.Fatalf("dispatch = %v, want [101]", h.disp.dispatch)
	}
	if len(h.safe.reserved) != 1 {
		t.Fatalf("reserved = %v, want one slot", h.safe.reserved)
	}
	if len(h.safe.released) != 0 || len(h.rec.calls) != 0 {
		t.Fatalf("clean dispatch must not release/recover: released=%v recovered=%v", h.safe.released, h.rec.calls)
	}
}

func TestBudgetZeroClaimsNothing(t *testing.T) {
	h := newHarness()
	h.safe.free = 0
	h.run(t)
	if len(h.claim.calls) != 0 || len(h.disp.dispatch) != 0 {
		t.Fatalf("budget=0 must not claim/dispatch: claims=%v dispatch=%v", h.claim.calls, h.disp.dispatch)
	}
}

func TestIneligibleAccountIsSkippedBeforeReadinessAndClaim(t *testing.T) {
	h := newHarness()
	h.safe.ineligible[101] = true
	h.run(t)
	if len(h.claim.calls) != 0 {
		t.Fatalf("parked account must not be claimed: claims=%v", h.claim.calls)
	}
}

func TestNotReadyAccountIsNotClaimed(t *testing.T) {
	h := newHarness()
	h.ready.notReady[101] = true
	h.run(t)
	if len(h.claim.calls) != 0 {
		t.Fatalf("not-ready account must not be claimed (retry-storm guard): claims=%v", h.claim.calls)
	}
}

func TestClaimMissReleasesReservationAndDoesNotDispatch(t *testing.T) {
	h := newHarness()
	h.claim.claimable = map[int64]PooledClaim{} // account eligible+ready but no queued run
	h.run(t)
	if len(h.claim.calls) != 1 {
		t.Fatalf("expected one claim attempt, got %v", h.claim.calls)
	}
	if len(h.safe.reserved) != 1 || len(h.safe.released) != 1 {
		t.Fatalf("claim miss must reserve then release (no leak): reserved=%v released=%v", h.safe.reserved, h.safe.released)
	}
	if len(h.disp.dispatch) != 0 {
		t.Fatalf("claim miss must not dispatch: dispatch=%v", h.disp.dispatch)
	}
	if h.safe.free != 1 {
		t.Fatalf("claim miss must restore the machine budget, free=%d want 1", h.safe.free)
	}
}

func TestClaimErrorReleasesReservationAndDoesNotDispatch(t *testing.T) {
	h := newHarness()
	h.claim.err = errors.New("pg blip")
	h.run(t)
	if len(h.safe.reserved) != 1 || len(h.safe.released) != 1 {
		t.Fatalf("claim error must reserve then release (no leak): reserved=%v released=%v", h.safe.reserved, h.safe.released)
	}
	if len(h.disp.dispatch) != 0 || len(h.rec.calls) != 0 {
		t.Fatalf("claim error must not dispatch or recover: dispatch=%v recover=%v", h.disp.dispatch, h.rec.calls)
	}
	if h.safe.free != 1 {
		t.Fatalf("claim error must restore the machine budget, free=%d want 1", h.safe.free)
	}
}

func TestDispatchFailureRecoversThenReleases(t *testing.T) {
	h := newHarness()
	h.disp.failFor[101] = errors.New("connector offline")
	h.run(t)
	if len(h.rec.calls) != 1 || h.rec.calls[0] != (RunFence{OrgID: 1, RunID: 9, Attempt: 1}) {
		t.Fatalf("dispatch failure must recover the exact fence, got %v", h.rec.calls)
	}
	if len(h.safe.released) != 1 {
		t.Fatalf("slot must be released after recovery commits, released=%v", h.safe.released)
	}
}

func TestRecoverFailureKeepsSlotHeld(t *testing.T) {
	h := newHarness()
	h.disp.failFor[101] = errors.New("connector offline")
	h.rec.err = errors.New("db down")
	h.run(t)
	if len(h.rec.calls) != 1 {
		t.Fatalf("expected one recover attempt, got %v", h.rec.calls)
	}
	if len(h.safe.released) != 0 {
		t.Fatalf("slot must NOT be released before the DB reflects the failure, released=%v", h.safe.released)
	}
}

func TestBudgetOneCapsAtOneDispatchAcrossAccounts(t *testing.T) {
	h := newHarness()
	h.pools.pools = []OrgPool{{OrgID: 1, AccountIDs: []int64{101, 102}}}
	h.claim.claimable[102] = PooledClaim{Fence: RunFence{RunID: 10, Attempt: 1}, CampaignID: 5, SourceID: 8}
	h.run(t)
	if len(h.disp.dispatch) != 1 {
		t.Fatalf("budget=1 with two claimable accounts must dispatch exactly once, got %v", h.disp.dispatch)
	}
	// Second account is never claimed because Reserve spent the only slot.
	for _, a := range h.claim.calls {
		if a == 102 {
			t.Fatalf("second account claimed despite spent budget: %v", h.claim.calls)
		}
	}
}

func TestPerOrgEnqueueErrorDoesNotAbortOtherOrgs(t *testing.T) {
	h := newHarness()
	h.pools.pools = []OrgPool{{OrgID: 1, AccountIDs: []int64{101}}, {OrgID: 2, AccountIDs: []int64{201}}}
	h.enq.errFor[1] = errors.New("enqueue boom")
	h.claim.claimable[201] = PooledClaim{Fence: RunFence{RunID: 11, Attempt: 1}, CampaignID: 6, SourceID: 9}
	h.run(t)
	if len(h.claim.calls) != 1 || h.claim.calls[0] != 201 {
		t.Fatalf("org 1 enqueue failure must not stop org 2: claims=%v", h.claim.calls)
	}
	if len(h.disp.dispatch) != 1 || h.disp.dispatch[0] != 201 {
		t.Fatalf("org 2 must still dispatch, got %v", h.disp.dispatch)
	}
}

func TestPoolReadErrorPropagates(t *testing.T) {
	h := newHarness()
	h.pools.err = errors.New("pg down")
	if err := h.orch.RunOnce(context.Background(), fixedNow); err == nil {
		t.Fatal("pool read error must propagate (fail closed), got nil")
	}
}
