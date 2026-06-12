package coordination_test

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// Admin assignment pause must flow from accounts.assignment_paused into
// the cap decision (gate #0) so both the queue gate and the readiness
// matrix report assignment_paused_by_admin (PR-2b).
func TestEvaluateCaps_AdminPauseBlocks(t *testing.T) {
	db, coord := newCoordinationStore(t, "assignment_pause.db")

	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Paused FB",
		AssignedUserID: 7, Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}

	d, err := coord.EvaluateCaps(context.Background(), accID, "comment")
	if err != nil {
		t.Fatalf("EvaluateCaps (fresh): %v", err)
	}
	if !d.Allowed {
		t.Fatalf("fresh account should be allowed, got reason=%q", d.Reason)
	}

	if err := db.Identities().SetAccountAssignmentPaused(accID, 1, true); err != nil {
		t.Fatalf("SetAccountAssignmentPaused: %v", err)
	}
	d, err = coord.EvaluateCaps(context.Background(), accID, "comment")
	if err != nil {
		t.Fatalf("EvaluateCaps (paused): %v", err)
	}
	if d.Allowed || d.Reason != "assignment_paused_by_admin" {
		t.Fatalf("paused account: got allowed=%v reason=%q, want blocked assignment_paused_by_admin", d.Allowed, d.Reason)
	}

	if err := db.Identities().SetAccountAssignmentPaused(accID, 1, false); err != nil {
		t.Fatalf("resume: %v", err)
	}
	d, _ = coord.EvaluateCaps(context.Background(), accID, "comment")
	if !d.Allowed {
		t.Fatalf("resumed account should be allowed again, got reason=%q", d.Reason)
	}
}

// Cross-org pause attempts must not mutate the row (tenant isolation).
func TestSetAccountAssignmentPaused_OrgScoped(t *testing.T) {
	db, _ := newCoordinationStore(t, "assignment_pause_org.db")
	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Org1 FB", Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	if err := db.Identities().SetAccountAssignmentPaused(accID, 2, true); err == nil {
		t.Fatalf("pausing with wrong org must fail")
	}
	paused, _ := db.Identities().AccountAssignmentPaused(accID)
	if paused {
		t.Fatalf("cross-org pause attempt must not flip the flag")
	}
}
