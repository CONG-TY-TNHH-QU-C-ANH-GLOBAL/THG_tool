// Domain: outbound (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// PR31B execution-core hardening, Domain 3: outbound final status
// transitions. Split from outbound_hardening_test.go at the documented
// domain seam (claim/lease/CAS stays there).

// TestFinalize_ExpiredClearsOutcome pins the ExecExpired terminal contract: an
// expired row never reached an observable state, so Finalize MUST drop any
// passed verification_outcome and store NULL. The existing suite only finalizes
// to ExecFinished.
func TestFinalize_ExpiredClearsOutcome(t *testing.T) {
	db := newSharedStore(t, "hard_finalize_expired.db")
	const org, acct int64 = 55, 7
	id := queueOnePlanned(t, db, org, acct, "https://facebook.com/p/expired")

	claim, err := db.Outbound().Claim(org, id, "worker", 0)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Pass a non-empty outcome on purpose — Finalize must ignore it for expired.
	finalized, state, outcome, _, err := db.Outbound().Finalize(
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
	if _, err := db.Outbound().Claim(org, id, "worker", 0); err != nil {
		t.Fatalf("claim: %v", err)
	}

	for _, bad := range []models.ExecutionState{models.ExecPlanned, models.ExecExecuting, ""} {
		t.Run(string(bad)+"_rejected", func(t *testing.T) {
			finalized, _, _, _, err := db.Outbound().Finalize(
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

	finalized, state, _, _, err := db.Outbound().Finalize(
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
	if _, err := db.Outbound().Claim(org, id, "worker", 0); err != nil {
		t.Fatalf("planned row must remain claimable after a no-op finalize: %v", err)
	}
}
