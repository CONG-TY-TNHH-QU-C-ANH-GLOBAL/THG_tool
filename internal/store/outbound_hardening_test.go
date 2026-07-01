// Domain: outbound (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

// PR31B execution-core hardening: behavior-preserving characterization across
// the three Execution Core domains (claim/lease/CAS, ledger, status transitions).

// ── Domain 1: Connector claim / lease / CAS ─────────────────────────────────

// TestClaim_ConcurrentDoubleClaimExactlyOnce pins the CAS concurrency primitive:
// when many workers race to claim the SAME planned row, exactly ONE wins and the
// rest are rejected. The existing suite only proves the SEQUENTIAL double-claim;
// the concurrent race (the real SW-restart / multi-worker case) was uncovered.
func TestClaim_ConcurrentDoubleClaimExactlyOnce(t *testing.T) {
	db := newSharedStore(t, "hard_claim_race.db")
	const org, acct int64 = 51, 7
	id := queueOnePlanned(t, db, org, acct, "https://facebook.com/p/claim-race")

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	claims := make([]*ClaimResult, workers)
	errs := make([]error, workers)
	for i := range workers {
		go func(idx int) {
			defer wg.Done()
			claims[idx], errs[idx] = db.ClaimPlannedOutboundForOrg(org, id, "worker", 0)
		}(i)
	}
	wg.Wait()

	won, execID := 0, ""
	for i := range workers {
		if errs[i] == nil && claims[i] != nil {
			won++
			execID = claims[i].ExecutionID
		}
	}
	if won != 1 {
		t.Fatalf("exactly one worker must win the claim CAS, got %d winners", won)
	}
	// The single winner's execution_id is the one stamped on the row.
	msg, err := db.Outbound().Get(org, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if msg.ExecutionState != models.ExecExecuting || msg.ExecutionID != execID {
		t.Fatalf("row must hold the winner's claim: state=%q exec=%q want executing/%q", msg.ExecutionState, msg.ExecutionID, execID)
	}
}

// TestClaim_WrongOrgRejected pins the cross-tenant defense in the claim CAS
// (org_id is part of the WHERE): a claim under a DIFFERENT org must NOT touch
// the row — it errors and the row stays planned for its owner.
func TestClaim_WrongOrgRejected(t *testing.T) {
	db := newSharedStore(t, "hard_claim_wrongorg.db")
	const ownerOrg, otherOrg, acct int64 = 52, 53, 7
	id := queueOnePlanned(t, db, ownerOrg, acct, "https://facebook.com/p/claim-tenant")

	if c, err := db.ClaimPlannedOutboundForOrg(otherOrg, id, "intruder", 0); err == nil || c != nil {
		t.Fatalf("cross-tenant claim must fail; got claim=%+v err=%v", c, err)
	}
	msg, err := db.Outbound().Get(ownerOrg, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if msg.ExecutionState != models.ExecPlanned || msg.ExecutionID != "" {
		t.Fatalf("owner row must stay planned/unclaimed; got state=%q exec=%q", msg.ExecutionState, msg.ExecutionID)
	}
}

// TestResetStale_LegacyNullLeaseFallback pins the legacy NULL-lease drain branch
// in ResetStaleExecuting: rows claimed before lease_expiry existed (lease NULL)
// must still be reset via the claimed_at + staleAfter window (the lease-aware
// test only covers the non-NULL path).
func TestResetStale_LegacyNullLeaseFallback(t *testing.T) {
	db := newSharedStore(t, "hard_reset_legacy.db")
	const org, acct int64 = 54, 7
	id := queueOnePlanned(t, db, org, acct, "https://facebook.com/p/reset-legacy")

	if _, err := db.ClaimPlannedOutboundForOrg(org, id, "worker", time.Hour); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Legacy shape: no lease column value, claimed_at well past the stale window.
	if _, err := db.db.Exec(
		`UPDATE outbound_messages SET lease_expiry = NULL, claimed_at = datetime('now', '-1 hour') WHERE id = ?`, id,
	); err != nil {
		t.Fatalf("force legacy shape: %v", err)
	}
	if err := db.ResetStaleExecutingForOrg(org, time.Minute); err != nil {
		t.Fatalf("reset: %v", err)
	}
	msg, err := db.Outbound().Get(org, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if msg.ExecutionState != models.ExecPlanned || msg.ExecutionID != "" {
		t.Fatalf("legacy NULL-lease stale row must reset to planned/cleared; got state=%q exec=%q", msg.ExecutionState, msg.ExecutionID)
	}
}

// ── Domain 3: Outbound final status transitions ─────────────────────────────

// TestFinalize_ExpiredClearsOutcome pins the ExecExpired terminal contract: an
// expired row never reached an observable state, so Finalize MUST drop any
// passed verification_outcome and store NULL. The existing suite only finalizes
// to ExecFinished.
func TestFinalize_ExpiredClearsOutcome(t *testing.T) {
	db := newSharedStore(t, "hard_finalize_expired.db")
	const org, acct int64 = 55, 7
	id := queueOnePlanned(t, db, org, acct, "https://facebook.com/p/expired")

	claim, err := db.ClaimPlannedOutboundForOrg(org, id, "worker", 0)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Pass a non-empty outcome on purpose — Finalize must ignore it for expired.
	finalized, state, outcome, _, err := db.FinalizeOutboundAttempt(
		context.Background(), org, id, claim.ExecutionID, models.ExecExpired, models.VerifVerifiedSuccess,
	)
	if err != nil || !finalized {
		t.Fatalf("expired finalize must succeed; finalized=%v err=%v", finalized, err)
	}
	if state != models.ExecExpired || outcome != "" {
		t.Fatalf("expired must return state=expired outcome=''; got state=%q outcome=%q", state, outcome)
	}
	// The stored column must be NULL (truthful "never observed").
	var stored any
	if err := db.db.QueryRow(`SELECT verification_outcome FROM outbound_messages WHERE id = ?`, id).Scan(&stored); err != nil {
		t.Fatalf("read outcome: %v", err)
	}
	if stored != nil {
		t.Fatalf("expired row verification_outcome must be NULL, got %v", stored)
	}
}

// TestFinalize_InvalidTerminalStateRejected pins the Finalize guard: only
// ExecFinished / ExecExpired are valid terminal states. Any other target is a
// programmer error and must be rejected before any DB mutation.
func TestFinalize_InvalidTerminalStateRejected(t *testing.T) {
	db := newSharedStore(t, "hard_finalize_badstate.db")
	const org, acct int64 = 56, 7
	id := queueOnePlanned(t, db, org, acct, "https://facebook.com/p/badstate")
	if _, err := db.ClaimPlannedOutboundForOrg(org, id, "worker", 0); err != nil {
		t.Fatalf("claim: %v", err)
	}

	for _, bad := range []models.ExecutionState{models.ExecPlanned, models.ExecExecuting, ""} {
		t.Run(string(bad)+"_rejected", func(t *testing.T) {
			finalized, _, _, _, err := db.FinalizeOutboundAttempt(
				context.Background(), org, id, "", bad, models.VerifVerifiedSuccess,
			)
			if err == nil || finalized {
				t.Fatalf("terminalState %q must be rejected; got finalized=%v err=%v", bad, finalized, err)
			}
		})
	}
	// The row must be untouched (still executing) after the rejected calls.
	msg, err := db.Outbound().Get(org, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if msg.ExecutionState != models.ExecExecuting {
		t.Fatalf("rejected finalize must not mutate the row; got state=%q", msg.ExecutionState)
	}
}

// TestFinalize_UnclaimedPlannedRowNotFinalized pins the CAS precondition: Finalize
// only acts on an EXECUTING row, so a finalize against a still-planned (never
// claimed) row must no-op (finalized=false) and leave it planned + re-claimable.
func TestFinalize_UnclaimedPlannedRowNotFinalized(t *testing.T) {
	db := newSharedStore(t, "hard_finalize_planned.db")
	const org, acct int64 = 57, 7
	id := queueOnePlanned(t, db, org, acct, "https://facebook.com/p/planned-finalize")

	finalized, state, _, _, err := db.FinalizeOutboundAttempt(
		context.Background(), org, id, "", models.ExecFinished, models.VerifVerifiedSuccess,
	)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if finalized {
		t.Fatal("a planned (unclaimed) row must NOT finalize")
	}
	if state != models.ExecPlanned {
		t.Fatalf("row must report current planned state; got %q", state)
	}
	// Still claimable afterwards.
	if _, err := db.ClaimPlannedOutboundForOrg(org, id, "worker", 0); err != nil {
		t.Fatalf("planned row must remain claimable after a no-op finalize: %v", err)
	}
}
