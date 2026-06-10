// Domain: coordination reverify fail-safe (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/coordination"
)

// scheduleAndClaimOne seeds a submitted_unverified comment, schedules + claims its reverify,
// and returns the comment_reverify row id.
func scheduleAndClaimOne(t *testing.T, db *Store, postURL string) int64 {
	t.Helper()
	ctx := context.Background()
	co := db.Coordination()
	seedSubmittedUnverified(t, db, 1, 10, postURL, "hello")
	jobs, _ := co.FindReverifyEligible(ctx, time.Now().Add(time.Hour), 50)
	for _, j := range jobs {
		_ = co.ScheduleReverify(ctx, j, time.Now())
	}
	claimed, _ := co.ClaimDueReverifies(ctx, 1, 10, 7, time.Now(), 10)
	if len(claimed) != 1 {
		t.Fatalf("claimed %d, want 1", len(claimed))
	}
	return claimed[0].ID
}

// A connector error records outcome=error (attempted_at set) — the job leaves pending and
// never appends a correction. Fixes the stuck pending+claimed+attempted-NULL state.
func TestReverify_ErrorLeavesNoStuckPending(t *testing.T) {
	db := newSharedStore(t, "reverify_error.db")
	ctx := context.Background()
	co := db.Coordination()
	postURL := "https://facebook.com/groups/1/posts/RVERR"
	rid := scheduleAndClaimOne(t, db, postURL)

	if err := co.RecordReverifyError(ctx, 1, rid, "content_unreachable:no_response"); err != nil {
		t.Fatalf("RecordReverifyError: %v", err)
	}
	// No correction appended.
	if outcomes := ledgerOutcomes(t, db, 1, postURL); len(outcomes) != 1 || outcomes[0] != models.LedgerOutcomeSubmittedUnverified {
		t.Errorf("ledger = %v, want only [submitted_unverified]", outcomes)
	}
	// Forensics reflects the error outcome (attempted, not stuck pending).
	rows, _ := co.CommentForensicsByTargetURLs(ctx, 1, []string{postURL})
	if rows[0].ReverifyOutcome != coordination.ReverifyError {
		t.Errorf("reverify_outcome = %q, want error", rows[0].ReverifyOutcome)
	}
	// Idempotent: a verdict on a resolved row is a no-op.
	if _, err := co.ApplyReverifyResult(ctx, 1, rid, true, "x", ""); err != nil {
		t.Fatalf("apply after error: %v", err)
	}
	if outcomes := ledgerOutcomes(t, db, 1, postURL); len(outcomes) != 1 {
		t.Errorf("resolved row must not accept a later correction: %v", outcomes)
	}
}

func ageClaimReverify(t *testing.T, db *Store, rid int64) {
	t.Helper()
	if _, err := db.db.Exec(`UPDATE comment_reverify SET claimed_at = datetime('now','-10 minutes') WHERE id = ?`, rid); err != nil {
		t.Fatalf("age claim: %v", err)
	}
}

// The lease-aware claim re-offers a pending job whose claim went stale (connector crashed
// before reporting), but does NOT re-offer a freshly-claimed one.
func TestReverify_LeaseReclaimsStaleClaim(t *testing.T) {
	db := newSharedStore(t, "reverify_lease.db")
	ctx := context.Background()
	co := db.Coordination()
	rid := scheduleAndClaimOne(t, db, "https://facebook.com/groups/1/posts/RVLEASE")

	// Freshly claimed → a second claim must NOT re-offer it (lease still held).
	again, _ := co.ClaimDueReverifies(ctx, 1, 10, 7, time.Now(), 10)
	if len(again) != 0 {
		t.Errorf("freshly-claimed job must not be re-offered, got %d", len(again))
	}

	ageClaimReverify(t, db, rid) // past the lease → re-claimable
	reclaimed, _ := co.ClaimDueReverifies(ctx, 1, 10, 7, time.Now(), 10)
	if len(reclaimed) != 1 || reclaimed[0].ID != rid {
		t.Errorf("stale-claimed job should be re-offered, got %+v", reclaimed)
	}
}

// Self-heal: a connector that keeps claiming (stale code) but never attempts gets the job
// retired as error=claim_without_attempt after the claim budget — never pending forever.
func TestReverify_ClaimWithoutAttemptSelfHeals(t *testing.T) {
	db := newSharedStore(t, "reverify_selfheal.db")
	ctx := context.Background()
	co := db.Coordination()
	postURL := "https://facebook.com/groups/1/posts/RVHEAL"
	rid := scheduleAndClaimOne(t, db, postURL) // claim #1

	// Reclaim until the budget is exhausted, never attempting.
	for i := 0; i < coordination.ReverifyMaxClaimsWithoutAttempt; i++ {
		ageClaimReverify(t, db, rid)
		if _, err := co.ClaimDueReverifies(ctx, 1, 10, 7, time.Now(), 10); err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
	}
	// The next claim self-heals it and stops re-offering.
	last, _ := co.ClaimDueReverifies(ctx, 1, 10, 7, time.Now(), 10)
	if len(last) != 0 {
		t.Errorf("a job over the claim budget must not be re-offered, got %d", len(last))
	}
	rows, _ := co.CommentForensicsByTargetURLs(ctx, 1, []string{postURL})
	if rows[0].ReverifyOutcome != coordination.ReverifyError {
		t.Errorf("reverify_outcome = %q, want error", rows[0].ReverifyOutcome)
	}
	if rows[0].ReverifyReason != coordination.ReverifyReasonClaimWithoutAttempt {
		t.Errorf("reverify_reason = %q, want claim_without_attempt", rows[0].ReverifyReason)
	}
}
