// Domain: outbound (see internal/store/DOMAINS.md)
package store

import (
	"sync"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/outbound"
)

// PR31B execution-core hardening: behavior-preserving characterization across
// the Execution Core domains. This file: claim/lease/CAS. Final status
// transitions live in outbound_hardening_finalize_test.go.

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
	claims := make([]*outbound.ClaimResult, workers)
	errs := make([]error, workers)
	for i := range workers {
		go func(idx int) {
			defer wg.Done()
			claims[idx], errs[idx] = db.Outbound().Claim(org, id, "worker", 0)
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

	if c, err := db.Outbound().Claim(otherOrg, id, "intruder", 0); err == nil || c != nil {
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

	if _, err := db.Outbound().Claim(org, id, "worker", time.Hour); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Legacy shape: no lease column value, claimed_at well past the stale window.
	if _, err := db.db.Exec(
		`UPDATE outbound_messages SET lease_expiry = NULL, claimed_at = datetime('now', '-1 hour') WHERE id = ?`, id,
	); err != nil {
		t.Fatalf("force legacy shape: %v", err)
	}
	if err := db.Outbound().ResetStaleExecuting(org, time.Minute); err != nil {
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
