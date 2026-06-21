package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func TestPostgresFinalizeTerminalCAS(t *testing.T) {
	store, pool := newTestStore(t)
	const org = int64(7)
	id := insertPlanned(t, pool, org, 11, "https://fb.com/p/3")

	claim, err := store.ClaimPlannedOutboundForOrg(org, id, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	finalized, state, outcome, execID, err := store.FinalizeOutboundAttempt(
		context.Background(), org, id, claim.ExecutionID, models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if !finalized || state != models.ExecFinished || outcome != models.VerifVerifiedSuccess || execID != claim.ExecutionID {
		t.Fatalf("first finalize wrong: finalized=%v state=%q outcome=%q execID=%q", finalized, state, outcome, execID)
	}

	// sent_at must be stamped for verified_success (timestamptz round-trip).
	finished, err := store.GetOutboundByExecutionStateForOrg(org, models.ExecFinished, "", 10)
	if err != nil {
		t.Fatalf("read finished: %v", err)
	}
	if len(finished) != 1 || finished[0].SentAt.IsZero() {
		t.Fatalf("verified_success must stamp a non-zero sent_at: %+v", finished)
	}
	if finished[0].VerificationOutcome != models.VerifVerifiedSuccess {
		t.Fatalf("verification_outcome should round-trip, got %q", finished[0].VerificationOutcome)
	}

	// Replay with the same token is idempotent: finalized=false, current state.
	replayed, curState, _, _, err := store.FinalizeOutboundAttempt(
		context.Background(), org, id, claim.ExecutionID, models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil {
		t.Fatalf("replay finalize: %v", err)
	}
	if replayed || curState != models.ExecFinished {
		t.Fatalf("replay should be finalized=false with current state finished, got finalized=%v state=%q", replayed, curState)
	}
}

func TestPostgresResetStaleExecuting(t *testing.T) {
	store, pool := newTestStore(t)
	const org = int64(7)
	id := insertPlanned(t, pool, org, 11, "https://fb.com/p/4")

	// Claim with a tiny lease so it is immediately past.
	claim, err := store.ClaimPlannedOutboundForOrg(org, id, "worker-a", time.Millisecond)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	if err := store.ResetStaleExecutingForOrg(org, time.Minute); err != nil {
		t.Fatalf("reset: %v", err)
	}

	planned, err := store.GetOutboundByExecutionStateForOrg(org, models.ExecPlanned, "", 10)
	if err != nil {
		t.Fatalf("read planned: %v", err)
	}
	if len(planned) != 1 {
		t.Fatalf("stale executing row should be reset to planned, got %d planned", len(planned))
	}
	// execution_id must be cleared so the abandoned attempt's report fails CAS.
	if planned[0].ExecutionID == claim.ExecutionID || planned[0].ExecutionID != "" {
		t.Fatalf("reset must clear execution_id, got %q", planned[0].ExecutionID)
	}
}

func TestPostgresOrgIsolation(t *testing.T) {
	store, pool := newTestStore(t)
	const orgA, orgB = int64(7), int64(8)
	id := insertPlanned(t, pool, orgA, 11, "https://fb.com/p/5")

	// orgB cannot see orgA's row.
	if rows, err := store.GetOutboundByExecutionStateForOrg(orgB, models.ExecPlanned, "", 10); err != nil || len(rows) != 0 {
		t.Fatalf("orgB must see no rows, got rows=%d err=%v", len(rows), err)
	}
	// orgB cannot claim orgA's row (cross-tenant CAS miss).
	if _, err := store.ClaimPlannedOutboundForOrg(orgB, id, "worker-b", time.Minute); err == nil {
		t.Fatalf("orgB claim of orgA row must fail")
	}
}
