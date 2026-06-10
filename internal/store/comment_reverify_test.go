// Domain: coordination reverify (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

// seedSubmittedUnverified queues a comment and forces it to the submitted_unverified
// terminal state (outbound 2-column + ledger), as the finalize path would.
func seedSubmittedUnverified(t *testing.T, db *Store, orgID, accountID int64, postURL, content string) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: orgID, Type: "comment", Platform: "facebook",
		AccountID: accountID, TargetURL: postURL, Content: content,
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if _, err := db.db.Exec(
		`UPDATE outbound_messages SET verification_outcome='submitted_unverified', execution_state='finished' WHERE id=?`,
		res.ID); err != nil {
		t.Fatalf("set submitted_unverified: %v", err)
	}
	if _, err := db.Coordination().MarkActionLedgerOutcomeByOutbound(ctx, orgID, res.ID, models.LedgerOutcomeSubmittedUnverified, "no node match"); err != nil {
		t.Fatalf("mark ledger: %v", err)
	}
	return res.ID
}

func ledgerOutcomes(t *testing.T, db *Store, orgID int64, url string) []string {
	t.Helper()
	entries, err := db.Coordination().ListActionLedger(context.Background(), orgID, "comment", url, time.Time{}, 50)
	if err != nil {
		t.Fatalf("ListActionLedger: %v", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Outcome)
	}
	return out
}

// #1 + #4: an eligible submitted_unverified is found + schedulable + claimable; a
// failed_before_submit (target_not_reached) is NOT eligible.
func TestReverify_EligibilityAndClaim(t *testing.T) {
	db := newSharedStore(t, "reverify_eligible.db")
	ctx := context.Background()
	co := db.Coordination()
	postURL := "https://facebook.com/groups/1/posts/RV1"
	outboundID := seedSubmittedUnverified(t, db, 1, 10, postURL, "Bên mình nhận sourcing")

	// A failed-before-submit comment must be ignored.
	failURL := "https://facebook.com/groups/1/posts/RVFAIL"
	failID, _ := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook", AccountID: 10, TargetURL: failURL, Content: "x",
	}, 24*time.Hour)
	_, _ = db.db.Exec(`UPDATE outbound_messages SET verification_outcome='target_not_reached', execution_state='finished' WHERE id=?`, failID.ID)

	future := time.Now().Add(time.Hour) // bypass the delay window for the test
	jobs, err := co.FindReverifyEligible(ctx, future, 50)
	if err != nil {
		t.Fatalf("FindReverifyEligible: %v", err)
	}
	var got, sawFail bool
	for _, j := range jobs {
		if j.OutboundID == outboundID {
			got = true
		}
		if j.OutboundID == failID.ID {
			sawFail = true
		}
	}
	if !got {
		t.Fatalf("submitted_unverified outbound %d should be eligible", outboundID)
	}
	if sawFail {
		t.Errorf("failed_before_submit outbound %d must NOT be eligible", failID.ID)
	}

	// Schedule + claim.
	for _, j := range jobs {
		if j.OutboundID == outboundID {
			if err := co.ScheduleReverify(ctx, j, time.Now()); err != nil {
				t.Fatalf("ScheduleReverify: %v", err)
			}
		}
	}
	claimed, err := co.ClaimDueReverifies(ctx, 1, 10, 7, time.Now(), 10)
	if err != nil {
		t.Fatalf("ClaimDueReverifies: %v", err)
	}
	if len(claimed) != 1 || claimed[0].OutboundID != outboundID {
		t.Fatalf("claimed = %+v, want 1 job for outbound %d", claimed, outboundID)
	}
}

// #2 + #5: a positive reverify appends a 'succeeded' correction (append-only — the old
// submitted_unverified row is untouched, a NEW row is added) and counts as a verified touch.
func TestReverify_FoundAppendsCorrectionAppendOnly(t *testing.T) {
	db := newSharedStore(t, "reverify_found.db")
	ctx := context.Background()
	co := db.Coordination()
	postURL := "https://facebook.com/groups/1/posts/RV2"
	seedSubmittedUnverified(t, db, 1, 10, postURL, "hi there")

	before := ledgerOutcomes(t, db, 1, postURL)
	if len(before) != 1 || before[0] != models.LedgerOutcomeSubmittedUnverified {
		t.Fatalf("setup ledger = %v, want [submitted_unverified]", before)
	}

	jobs, _ := co.FindReverifyEligible(ctx, time.Now().Add(time.Hour), 50)
	for _, j := range jobs {
		_ = co.ScheduleReverify(ctx, j, time.Now())
	}
	claimed, _ := co.ClaimDueReverifies(ctx, 1, 10, 7, time.Now(), 10)
	rid := claimed[0].ID

	corrected, err := co.ApplyReverifyResult(ctx, 1, rid, true, "https://facebook.com/x?comment_id=999", "found")
	if err != nil {
		t.Fatalf("ApplyReverifyResult: %v", err)
	}
	if !corrected {
		t.Fatalf("found reverify should append a correction")
	}

	after := ledgerOutcomes(t, db, 1, postURL)
	if len(after) != 2 {
		t.Fatalf("ledger rows = %d (%v), want 2 (append-only: old + correction)", len(after), after)
	}
	hasSubmitted, hasSucceeded := false, false
	for _, o := range after {
		if o == models.LedgerOutcomeSubmittedUnverified {
			hasSubmitted = true
		}
		if o == "succeeded" {
			hasSucceeded = true
		}
	}
	if !hasSubmitted {
		t.Errorf("original submitted_unverified row must remain (not mutated)")
	}
	if !hasSucceeded {
		t.Errorf("a 'succeeded' correction row must be appended")
	}
	if !models.IsLedgerOutcomeVerifiedTouch("succeeded") {
		t.Errorf("the correction must count as a verified touch")
	}

	// Idempotent: applying again is a no-op (row already resolved).
	again, _ := co.ApplyReverifyResult(ctx, 1, rid, true, "", "")
	if again {
		t.Errorf("second apply on a resolved row must be a no-op")
	}
}

// #3: a negative reverify keeps submitted_unverified (no correction) and records not_found.
func TestReverify_NotFoundStaysUnverified(t *testing.T) {
	db := newSharedStore(t, "reverify_notfound.db")
	ctx := context.Background()
	co := db.Coordination()
	postURL := "https://facebook.com/groups/1/posts/RV3"
	seedSubmittedUnverified(t, db, 1, 10, postURL, "hello")

	jobs, _ := co.FindReverifyEligible(ctx, time.Now().Add(time.Hour), 50)
	for _, j := range jobs {
		_ = co.ScheduleReverify(ctx, j, time.Now())
	}
	claimed, _ := co.ClaimDueReverifies(ctx, 1, 10, 7, time.Now(), 10)

	corrected, err := co.ApplyReverifyResult(ctx, 1, claimed[0].ID, false, "", "comment not visible")
	if err != nil {
		t.Fatalf("ApplyReverifyResult: %v", err)
	}
	if corrected {
		t.Errorf("not-found reverify must NOT append a correction")
	}
	after := ledgerOutcomes(t, db, 1, postURL)
	if len(after) != 1 || after[0] != models.LedgerOutcomeSubmittedUnverified {
		t.Errorf("ledger must stay [submitted_unverified], got %v", after)
	}
}
