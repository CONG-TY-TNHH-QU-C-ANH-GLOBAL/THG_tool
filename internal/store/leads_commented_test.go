// Domain: leads (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

// GetLeadsFiltered.commented reads VERIFIED action_ledger truth (outcome='succeeded'),
// not the legacy outbound_messages.status enum. A queued-but-unverified comment must NOT
// flip commented; a verified one must. Guards the bug fix (item 4 closeout) against drift.
func TestGetLeadsFiltered_CommentedFromVerifiedLedger(t *testing.T) {
	db := newSharedStore(t, "commented_truth.db")
	ctx := context.Background()
	postURL := "https://facebook.com/post/CMT"
	leadID := seedListableLead(t, db, 1, postURL, "https://facebook.com/profile/CMT")

	commentedFor := func(id int64) bool {
		list, err := db.Leads().GetLeadsFiltered("", "", 50, 0, 1)
		if err != nil {
			t.Fatalf("GetLeadsFiltered: %v", err)
		}
		for _, l := range list {
			if l.ID == id {
				return l.Commented
			}
		}
		t.Fatalf("lead %d missing from list", id)
		return false
	}

	// Queue only → ledger outcome 'queued' → NOT a verified touch → commented stays false.
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: 10, TargetURL: postURL, Content: "hi",
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if commentedFor(leadID) {
		t.Errorf("queued-but-unverified comment must NOT mark lead commented")
	}

	// Verify → outcome 'succeeded' → commented flips true.
	if _, err := db.Coordination().MarkActionLedgerOutcomeByOutbound(ctx, 1, res.ID, "succeeded", "verified by test"); err != nil {
		t.Fatalf("mark verified: %v", err)
	}
	if !commentedFor(leadID) {
		t.Errorf("verified comment must mark lead commented")
	}
}
