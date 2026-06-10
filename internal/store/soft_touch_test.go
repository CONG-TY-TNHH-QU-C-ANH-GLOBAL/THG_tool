// Domain: leads (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

// markLedger queues a comment on the lead's source URL and forces a terminal ledger
// outcome, returning the outbound id. Mirrors the real finalize path's ledger write.
func markLedger(t *testing.T, db *Store, orgID, accountID int64, postURL, outcome string) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: orgID, Type: "comment", Platform: "facebook",
		AccountID: accountID, TargetURL: postURL, Content: "hi",
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if _, err := db.Coordination().MarkActionLedgerOutcomeByOutbound(ctx, orgID, res.ID, outcome, "test"); err != nil {
		t.Fatalf("mark %s: %v", outcome, err)
	}
	return res.ID
}

// Test #1 + #3 + #4: a submitted_unverified comment makes the lead waiting_verification,
// which the work queue excludes by default (blocks immediate re-comment) but surfaces when
// the state is explicitly requested (reverify/retry path).
func TestSoftTouch_BlocksWorkQueueUntilVerified(t *testing.T) {
	db := newSharedStore(t, "soft_touch_block.db")
	ctx := context.Background()
	postURL := "https://facebook.com/groups/1/posts/SOFT1"
	leadID := seedListableLead(t, db, 1, postURL, "https://facebook.com/profile/SOFT1")
	markLedger(t, db, 1, 10, postURL, models.LedgerOutcomeSubmittedUnverified)

	// Lifecycle: waiting_verification (not active, not succeeded).
	lc, err := db.Leads().GetLeadLifecycle(ctx, 1, leadID, models.DefaultLeadLifecyclePolicy())
	if err != nil {
		t.Fatalf("GetLeadLifecycle: %v", err)
	}
	if lc.FreshnessState != models.LeadWaitingVerification {
		t.Fatalf("state = %s, want waiting_verification", lc.FreshnessState)
	}

	// Default work queue (and planner path) exclude it → blocks immediate re-comment.
	def, err := db.Leads().WorkQueueLeads(ctx, 1, "", 50)
	if err != nil {
		t.Fatalf("WorkQueueLeads: %v", err)
	}
	if containsLeadID(def, leadID) {
		t.Errorf("waiting_verification lead %d must be excluded from the planner work queue", leadID)
	}

	// Explicit request for the state surfaces it (reverify/retry path).
	items, err := db.Leads().GetWorkQueue(ctx, 1, models.WorkQueueOptions{
		States: []models.LeadFreshnessState{models.LeadWaitingVerification},
		Limit:  50,
	})
	if err != nil {
		t.Fatalf("GetWorkQueue explicit: %v", err)
	}
	found := false
	for _, it := range items {
		if it.Lead.ID == leadID {
			found = true
		}
	}
	if !found {
		t.Errorf("explicit waiting_verification request should surface lead %d", leadID)
	}
}

// Test #2: a failure BEFORE submit (ledger 'failed') is not a soft touch — the lead stays
// eligible in the work queue (a fresh retry is safe).
func TestSoftTouch_FailedBeforeSubmitDoesNotBlock(t *testing.T) {
	db := newSharedStore(t, "soft_touch_failed.db")
	ctx := context.Background()
	postURL := "https://facebook.com/groups/1/posts/FAIL1"
	leadID := seedListableLead(t, db, 1, postURL, "https://facebook.com/profile/FAIL1")
	markLedger(t, db, 1, 10, postURL, "failed")

	lc, err := db.Leads().GetLeadLifecycle(ctx, 1, leadID, models.DefaultLeadLifecyclePolicy())
	if err != nil {
		t.Fatalf("GetLeadLifecycle: %v", err)
	}
	if lc.FreshnessState != models.LeadActive {
		t.Errorf("failed-before-submit lead state = %s, want active (retry eligible)", lc.FreshnessState)
	}
	leads, err := db.Leads().WorkQueueLeads(ctx, 1, "", 50)
	if err != nil {
		t.Fatalf("WorkQueueLeads: %v", err)
	}
	if !containsLeadID(leads, leadID) {
		t.Errorf("failed-before-submit lead %d must remain eligible in the work queue", leadID)
	}
}

// Test #5: a verified success (succeeded) is a HARD touch — lifecycle reflects engagement,
// not waiting_verification.
func TestSoftTouch_VerifiedSuccessIsHardTouch(t *testing.T) {
	db := newSharedStore(t, "soft_touch_verified.db")
	ctx := context.Background()
	postURL := "https://facebook.com/groups/1/posts/OK1"
	leadID := seedListableLead(t, db, 1, postURL, "https://facebook.com/profile/OK1")
	markLedger(t, db, 1, 10, postURL, "succeeded")

	lc, err := db.Leads().GetLeadLifecycle(ctx, 1, leadID, models.DefaultLeadLifecyclePolicy())
	if err != nil {
		t.Fatalf("GetLeadLifecycle: %v", err)
	}
	// A verified comment with no reply, inside the followup window → waiting_reply (engaged),
	// NOT waiting_verification and NOT active-untouched.
	if lc.FreshnessState == models.LeadWaitingVerification || lc.FreshnessState == models.LeadActive {
		t.Errorf("verified success should be a hard touch, got %s", lc.FreshnessState)
	}
	if lc.LastEngagedAt.IsZero() {
		t.Errorf("verified success must set last_engaged_at (hard touch)")
	}
}
