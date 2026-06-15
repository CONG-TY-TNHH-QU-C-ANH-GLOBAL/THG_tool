package coordination_test

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store/coordination"
)

func mkInput(orgID, accountID, userID int64, canonical string) coordination.DirectPostWorkflowInput {
	return coordination.DirectPostWorkflowInput{
		OrgID: orgID, RequestedByUserID: userID, UserRole: "sales", AccountID: accountID,
		CanonicalPostURL: canonical, PostFBID: "456", GroupRef: "ship.viet.my", Prompt: "comment bài này",
	}
}

// Create/get + the two-key idempotency model: one comment-workflow request per actor
// (idempotency_key UNIQUE), one shared import per post (intake_key).
func TestDirectPostWorkflow_CreateAndIdempotency(t *testing.T) {
	ctx := context.Background()
	_, c := newCoordinationStore(t, "dpw_idem")
	const org = int64(7)
	const canonical = "https://www.facebook.com/groups/ship.viet.my/permalink/456/"

	w1, err := c.CreateOrGetDirectPostCommentWorkflow(ctx, mkInput(org, 3, 99, canonical))
	if err != nil || w1 == nil {
		t.Fatalf("create: %v", err)
	}
	if w1.Status != coordination.DPStatusRequested {
		t.Errorf("new workflow status = %q, want requested", w1.Status)
	}
	// Same actor/action/post → SAME row (no duplicate workflow).
	w2, err := c.CreateOrGetDirectPostCommentWorkflow(ctx, mkInput(org, 3, 99, canonical))
	if err != nil || w2 == nil || w2.ID != w1.ID {
		t.Fatalf("idempotent create returned a different row: w1=%d w2=%v err=%v", w1.ID, w2, err)
	}
	// Different acting account → DIFFERENT workflow, but SAME shared intake_key.
	w3, err := c.CreateOrGetDirectPostCommentWorkflow(ctx, mkInput(org, 5, 99, canonical))
	if err != nil || w3 == nil {
		t.Fatalf("create w3: %v", err)
	}
	if w3.ID == w1.ID {
		t.Errorf("different account must create a distinct workflow")
	}
	if w3.IntakeKey != w1.IntakeKey {
		t.Errorf("same post must share intake_key: %q vs %q", w3.IntakeKey, w1.IntakeKey)
	}
	if w3.IdempotencyKey == w1.IdempotencyKey {
		t.Errorf("different account must have a distinct idempotency_key")
	}
	// FindActiveByIntakeKey finds an in-flight import for the post.
	found, err := c.FindActiveDirectPostCommentWorkflowByIntakeKey(ctx, org, w1.IntakeKey)
	if err != nil || found == nil {
		t.Fatalf("find by intake key: %v", err)
	}
	// Org isolation.
	if other, _ := c.GetDirectPostCommentWorkflowByID(ctx, 8, w1.ID); other != nil {
		t.Errorf("org 8 must not read org 7's workflow")
	}
}

// CAS transitions only fire from the expected prior status; a stale repeat is a clean
// false, never a clobber.
func TestDirectPostWorkflow_CASTransitions(t *testing.T) {
	ctx := context.Background()
	_, c := newCoordinationStore(t, "dpw_cas")
	const org = int64(7)
	w, err := c.CreateOrGetDirectPostCommentWorkflow(ctx, mkInput(org, 3, 99, "https://www.facebook.com/groups/g/permalink/1/"))
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := c.MarkDirectPostImportQueued(ctx, org, w.ID, "task-1"); err != nil || !ok {
		t.Fatalf("import_queued from requested should succeed: ok=%v err=%v", ok, err)
	}
	// Stale repeat (now import_queued, not requested) → CAS false.
	if ok, _ := c.MarkDirectPostImportQueued(ctx, org, w.ID, "task-1"); ok {
		t.Error("import_queued must NOT re-fire from import_queued (stale CAS)")
	}
	// Wrong org → false.
	if ok, _ := c.MarkDirectPostLeadCreated(ctx, 8, w.ID, 555); ok {
		t.Error("cross-org transition must fail")
	}
	if ok, err := c.MarkDirectPostLeadCreated(ctx, org, w.ID, 555); err != nil || !ok {
		t.Fatalf("lead_created from import_queued should succeed: ok=%v err=%v", ok, err)
	}
	if ok, err := c.MarkDirectPostCommentQueued(ctx, org, w.ID); err != nil || !ok {
		t.Fatalf("comment_queued from lead_created should succeed: ok=%v err=%v", ok, err)
	}
	if ok, err := c.MarkDirectPostCompleted(ctx, org, w.ID); err != nil || !ok {
		t.Fatalf("completed from comment_queued should succeed: ok=%v err=%v", ok, err)
	}
	got, _ := c.GetDirectPostCommentWorkflowByID(ctx, org, w.ID)
	if got == nil || got.Status != coordination.DPStatusCompleted || !got.LeadID.Valid || got.LeadID.Int64 != 555 {
		t.Errorf("final state wrong: %+v", got)
	}
	// Terminal: completed must not fail.
	if ok, _ := c.MarkDirectPostFailed(ctx, org, w.ID, "x", "y"); ok {
		t.Error("completed workflow must not transition to failed")
	}
}

// Retry scheduling updates next_run_at + retry_count; the row is re-claimable after.
func TestDirectPostWorkflow_RetryAndClaim(t *testing.T) {
	ctx := context.Background()
	_, c := newCoordinationStore(t, "dpw_retry")
	const org = int64(7)
	w, err := c.CreateOrGetDirectPostCommentWorkflow(ctx, mkInput(org, 3, 99, "https://www.facebook.com/groups/g/permalink/2/"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	// Claim due workflows (new rows are 'requested', next_run_at=now → due).
	claimed, err := c.ClaimDueDirectPostCommentWorkflows(ctx, now.Add(time.Second), "worker-a", now.Add(coordination.DPDefaultLeaseDuration), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != w.ID {
		t.Fatalf("expected to claim w, got %d rows", len(claimed))
	}
	// A second worker within the lease must NOT double-claim.
	again, _ := c.ClaimDueDirectPostCommentWorkflows(ctx, now.Add(2*time.Second), "worker-b", now.Add(coordination.DPDefaultLeaseDuration), 10)
	if len(again) != 0 {
		t.Errorf("leased workflow must not be double-claimed, got %d", len(again))
	}
	// Schedule a retry (e.g. login_required) for the future → released + counted.
	if ok, err := c.ScheduleDirectPostRetry(ctx, org, w.ID, now.Add(time.Hour), coordination.DPStatusLoginRequired, "needs login"); err != nil || !ok {
		t.Fatalf("schedule retry: ok=%v err=%v", ok, err)
	}
	got, _ := c.GetDirectPostCommentWorkflowByID(ctx, org, w.ID)
	if got.Status != coordination.DPStatusRetryScheduled || got.RetryCount != 1 || got.ErrorCode != coordination.DPStatusLoginRequired {
		t.Errorf("retry state wrong: %+v", got)
	}
	// Not due yet (next_run_at in the future) → not claimed.
	notYet, _ := c.ClaimDueDirectPostCommentWorkflows(ctx, now.Add(3*time.Second), "worker-c", now.Add(coordination.DPDefaultLeaseDuration), 10)
	if len(notYet) != 0 {
		t.Errorf("retry-scheduled-for-later must not be claimed yet, got %d", len(notYet))
	}
}

// A claim whose lease has EXPIRED becomes reclaimable by another worker (the first
// poller crashed before reporting) — but NOT before the lease elapses.
func TestDirectPostWorkflow_ExpiredLeaseReclaim(t *testing.T) {
	ctx := context.Background()
	_, c := newCoordinationStore(t, "dpw_expired_lease")
	const org = int64(7)
	w, err := c.CreateOrGetDirectPostCommentWorkflow(ctx, mkInput(org, 3, 99, "https://www.facebook.com/groups/g/permalink/3/"))
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC()

	// Claim with a SHORT lease expiring at base+10s.
	claimed, err := c.ClaimDueDirectPostCommentWorkflows(ctx, base, "worker-a", base.Add(10*time.Second), 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("first claim: %d rows err=%v", len(claimed), err)
	}
	// Within the lease (base+5s) → not reclaimable.
	if within, _ := c.ClaimDueDirectPostCommentWorkflows(ctx, base.Add(5*time.Second), "worker-b", base.Add(time.Minute), 10); len(within) != 0 {
		t.Errorf("within an unexpired lease must not reclaim, got %d", len(within))
	}
	// After the lease expires (base+11s) → reclaimable by a different worker.
	reclaimed, err := c.ClaimDueDirectPostCommentWorkflows(ctx, base.Add(11*time.Second), "worker-c", base.Add(2*time.Minute), 10)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].ID != w.ID {
		t.Fatalf("expired lease must be reclaimable: %d rows err=%v", len(reclaimed), err)
	}
}