package main

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
)

// seedImportQueuedWorkflow creates a workflow already in import_queued (via the store,
// no jobStore) so the poller tests isolate the continuation logic.
func seedImportQueuedWorkflow(t *testing.T, ctx context.Context, db *store.Store, org, user int64) *coordination.DirectPostCommentWorkflow {
	t.Helper()
	w, err := db.Coordination().CreateOrGetDirectPostCommentWorkflow(ctx, coordination.DirectPostWorkflowInput{
		OrgID: org, RequestedByUserID: user, UserRole: "sales", CanonicalPostURL: intakeCanonical, PostFBID: intakePostFBID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := db.Coordination().MarkDirectPostImportQueued(ctx, org, w.ID, "task-seed"); err != nil || !ok {
		t.Fatalf("seed import_queued: ok=%v err=%v", ok, err)
	}
	w, _ = db.Coordination().GetDirectPostCommentWorkflowByID(ctx, org, w.ID)
	return w
}

// D — continuation: once the post lead exists, the poller advances to completed and
// queues the comment exactly once.
func TestDirectPostIntake_ContinuationOnLead(t *testing.T) {
	ctx := context.Background()
	db := newIntakeDB(t)
	const org, user int64 = 5, 9
	// The requester owns an account (execution-layer ownership) so the comment can be
	// queued as them; with no ready connector it reaches the readiness gate cleanly.
	if _, err := db.Identities().AddAccount(&models.Account{OrgID: org, Platform: models.PlatformFacebook, Name: "acc", AssignedUserID: user, Status: models.AccountActive}); err != nil {
		t.Fatal(err)
	}
	w := seedImportQueuedWorkflow(t, ctx, db, org, user)
	if _, err := db.Leads().InsertLead(&models.Lead{
		OrgID: org, SourceType: "post", SourceURL: intakeCanonical, PostFBID: intakePostFBID, GroupFBID: "123",
		Platform: models.PlatformFacebook, Author: "An", Content: "ai làm fulfill US", Score: models.LeadHot,
	}); err != nil {
		t.Fatal(err)
	}

	advanceDirectPostWorkflow(ctx, db, ai.NewMessageGenerator("", ""), nil, w)
	got, _ := db.Coordination().GetDirectPostCommentWorkflowByID(ctx, org, w.ID)
	if got.Status != coordination.DPStatusCompleted || !got.LeadID.Valid {
		t.Errorf("workflow should be completed with a lead_id after continuation: status=%s lead=%v err=%s", got.Status, got.LeadID, got.ErrorCode)
	}
	// Idempotent: a second advance on the now-completed workflow is a clean no-op.
	advanceDirectPostWorkflow(ctx, db, ai.NewMessageGenerator("", ""), nil, w)
	if again, _ := db.Coordination().GetDirectPostCommentWorkflowByID(ctx, org, w.ID); again.Status != coordination.DPStatusCompleted {
		t.Errorf("re-advance must not change a completed workflow, got %s", again.Status)
	}
}

// F — import not visible: bounded retry, then a typed terminal failure.
func TestDirectPostIntake_RetryThenImportFailed(t *testing.T) {
	ctx := context.Background()
	db := newIntakeDB(t)
	const org int64 = 7
	w := seedImportQueuedWorkflow(t, ctx, db, org, 9)

	// First miss → schedule a retry (not failed).
	advanceDirectPostWorkflow(ctx, db, ai.NewMessageGenerator("", ""), nil, w)
	got, _ := db.Coordination().GetDirectPostCommentWorkflowByID(ctx, org, w.ID)
	if got.Status != coordination.DPStatusRetryScheduled || got.RetryCount != 1 {
		t.Fatalf("first miss should schedule a retry: status=%s n=%d", got.Status, got.RetryCount)
	}
	// Drive retry_count to the cap, then advance once more with no lead → terminal.
	for got.RetryCount < coordination.DPMaxRetryCount {
		_, _ = db.Coordination().ScheduleDirectPostRetry(ctx, org, w.ID, time.Now().Add(time.Minute), coordination.DPStatusImporting, "x")
		got, _ = db.Coordination().GetDirectPostCommentWorkflowByID(ctx, org, w.ID)
	}
	advanceDirectPostWorkflow(ctx, db, ai.NewMessageGenerator("", ""), nil, got)
	final, _ := db.Coordination().GetDirectPostCommentWorkflowByID(ctx, org, w.ID)
	if final.Status != coordination.DPStatusFailed || final.ErrorCode != coordination.DPErrLeadNotObserved {
		t.Errorf("at max retries with no lead → failed(lead_not_observed_after_retries), got status=%s code=%s", final.Status, final.ErrorCode)
	}
}

// H — graceful shutdown: the scheduler exits promptly on context cancellation.
func TestDirectPostIntake_GracefulShutdown(t *testing.T) {
	db := newIntakeDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		runDirectPostIntakeScheduler(ctx, db, ai.NewMessageGenerator("", ""), nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not exit promptly on context cancellation")
	}
}
