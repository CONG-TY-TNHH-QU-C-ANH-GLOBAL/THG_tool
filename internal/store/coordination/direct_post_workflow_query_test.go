package coordination_test

import (
	"context"
	"testing"
)

// FindDirectPostWorkflowByImportTaskID is the durable provenance link the crawl-result
// ingest uses to recognise an explicit direct-post import (no in-memory state). It must
// resolve the workflow by import_task_id, be org-scoped, and return (nil,nil) for an
// unknown task.
func TestFindDirectPostWorkflowByImportTaskID(t *testing.T) {
	ctx := context.Background()
	_, c := newCoordinationStore(t, "dpw_by_task")
	const org = int64(7)
	const canonical = "https://www.facebook.com/groups/ship.viet.my/permalink/456/"

	w, err := c.CreateOrGetDirectPostCommentWorkflow(ctx, mkInput(org, 3, 99, canonical))
	if err != nil || w == nil {
		t.Fatalf("create: %v", err)
	}
	const taskID = "open-crawl-deadbeefcafe"
	if ok, err := c.MarkDirectPostImportQueued(ctx, org, w.ID, taskID); err != nil || !ok {
		t.Fatalf("mark import queued: ok=%v err=%v", ok, err)
	}

	got, err := c.FindDirectPostWorkflowByImportTaskID(ctx, org, taskID)
	if err != nil || got == nil {
		t.Fatalf("lookup by import_task_id: got=%v err=%v", got, err)
	}
	if got.ID != w.ID || got.CanonicalPostURL != canonical || got.GroupRef != "ship.viet.my" {
		t.Errorf("lookup returned wrong/lossy workflow: %+v", got)
	}

	// Unknown task id → (nil, nil).
	if other, _ := c.FindDirectPostWorkflowByImportTaskID(ctx, org, "open-crawl-unknown"); other != nil {
		t.Errorf("unknown task must return nil, got id=%d", other.ID)
	}
	// Org isolation.
	if other, _ := c.FindDirectPostWorkflowByImportTaskID(ctx, 8, taskID); other != nil {
		t.Errorf("org 8 must not resolve org 7's workflow")
	}
}
