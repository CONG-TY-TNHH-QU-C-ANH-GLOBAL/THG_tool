package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
	"github.com/thg/scraper/internal/store/sessions"
)

const intakeCanonical = "https://www.facebook.com/groups/ship.viet.my/permalink/4504452536547584/"
const intakePostFBID = "4504452536547584"

// seedDispatchableSession makes the P1.3C account-pinned import dispatchable in tests: the
// import is now pinned to the action account, so submitConnectorCrawl needs a usable session
// for that account to fall through to the worker queue (routed=false). In production the
// action account is online; here we seed an idle CDP session per account.
func seedDispatchableSession(t *testing.T, db *store.Store, orgID, accountID int64) {
	t.Helper()
	if err := db.Sessions().UpsertSession(context.Background(), sessions.BrowserSession{
		AccountID: accountID, OrgID: orgID, Status: "idle", CDPPort: 9222,
		StartedAt: time.Now().UTC(), LastActiveAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func newIntakeDB(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "intake.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// newIntakeEnv adds a jobStore (second SQLite handle) for the import-enqueue tests.
func newIntakeEnv(t *testing.T) (*store.Store, *jobs.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "intake.db")
	db, err := store.New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	js, err := jobs.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = js.Close() })
	return db, js
}

func countImportJobs(t *testing.T, js *jobs.Store) int {
	t.Helper()
	all, _ := js.List(context.Background(), "", 200)
	n := 0
	for _, j := range all {
		if j.Intent == "facebook_crawl" {
			n++
		}
	}
	return n
}

// B — unknown post: durable workflow + exactly one facebook_post import + async ack.
func TestDirectPostIntake_UnknownPost(t *testing.T) {
	ctx := context.Background()
	db, js := newIntakeEnv(t)
	intake := newDirectPostIntake(db, js)
	const org, user, acct int64 = 7, 99, 3
	seedDispatchableSession(t, db, org, acct)

	msg, err := intake.request(ctx, directPostCommentInput{
		OrgID: org, RequestedByUserID: user, AccountID: acct, UserRole: "sales",
		CanonicalPostURL: intakeCanonical, PostFBID: intakePostFBID, Prompt: "comment bài này",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "CHỈ comment nếu xác minh") || strings.Contains(msg, "Hãy quét/import") {
		t.Errorf("unknown post must return the async intake ack, got %q", msg)
	}
	w, _ := db.Coordination().GetDirectPostCommentWorkflowByIdempotencyKey(ctx, org,
		coordination.DirectPostIdempotencyKey(org, acct, user, intakeCanonical))
	if w == nil || w.Status != coordination.DPStatusImportQueued || w.ImportTaskID == "" {
		t.Fatalf("workflow not in import_queued with a task id: %+v", w)
	}
	if countImportJobs(t, js) != 1 {
		t.Errorf("expected exactly one facebook_post import job, got %d", countImportJobs(t, js))
	}
}

// C — same post, two actors: one shared import, two distinct workflows.
func TestDirectPostIntake_MultiActorSharesImport(t *testing.T) {
	ctx := context.Background()
	db, js := newIntakeEnv(t)
	intake := newDirectPostIntake(db, js)
	const org int64 = 7
	seedDispatchableSession(t, db, org, 3)
	seedDispatchableSession(t, db, org, 5)
	base := directPostCommentInput{OrgID: org, UserRole: "sales", CanonicalPostURL: intakeCanonical, PostFBID: intakePostFBID, Prompt: "comment bài này"}

	a := base
	a.RequestedByUserID, a.AccountID = 99, 3
	if _, err := intake.request(ctx, a); err != nil {
		t.Fatal(err)
	}
	b := base
	b.RequestedByUserID, b.AccountID = 100, 5
	if _, err := intake.request(ctx, b); err != nil {
		t.Fatal(err)
	}

	wa, _ := db.Coordination().GetDirectPostCommentWorkflowByIdempotencyKey(ctx, org, coordination.DirectPostIdempotencyKey(org, 3, 99, intakeCanonical))
	wb, _ := db.Coordination().GetDirectPostCommentWorkflowByIdempotencyKey(ctx, org, coordination.DirectPostIdempotencyKey(org, 5, 100, intakeCanonical))
	if wa == nil || wb == nil || wa.ID == wb.ID {
		t.Fatalf("expected two distinct workflows, wa=%v wb=%v", wa, wb)
	}
	if wa.ImportTaskID == "" || wa.ImportTaskID != wb.ImportTaskID {
		t.Errorf("multi-actor must SHARE one import task: wa=%q wb=%q", wa.ImportTaskID, wb.ImportTaskID)
	}
	if countImportJobs(t, js) != 1 {
		t.Errorf("expected exactly ONE import job for the intake_key, got %d", countImportJobs(t, js))
	}
}

// E — idempotency: same actor/prompt twice reuses one workflow + one import.
func TestDirectPostIntake_IdempotentRepeat(t *testing.T) {
	ctx := context.Background()
	db, js := newIntakeEnv(t)
	intake := newDirectPostIntake(db, js)
	seedDispatchableSession(t, db, 7, 3)
	in := directPostCommentInput{OrgID: 7, RequestedByUserID: 99, AccountID: 3, UserRole: "sales", CanonicalPostURL: intakeCanonical, PostFBID: intakePostFBID, Prompt: "comment bài này"}
	if _, err := intake.request(ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, err := intake.request(ctx, in); err != nil {
		t.Fatal(err)
	}
	if countImportJobs(t, js) != 1 {
		t.Errorf("repeat request must not duplicate the import job, got %d", countImportJobs(t, js))
	}
}

// Re-prompt after a TERMINAL failure re-opens the workflow (status → requested →
// import_queued with a fresh import) instead of dead-ending on a lying ack.
func TestDirectPostIntake_ReRequestAfterFailed(t *testing.T) {
	ctx := context.Background()
	db, js := newIntakeEnv(t)
	intake := newDirectPostIntake(db, js)
	seedDispatchableSession(t, db, 7, 3)
	in := directPostCommentInput{OrgID: 7, RequestedByUserID: 99, AccountID: 3, UserRole: "sales", CanonicalPostURL: intakeCanonical, PostFBID: intakePostFBID, Prompt: "comment bài này"}
	if _, err := intake.request(ctx, in); err != nil {
		t.Fatal(err)
	}
	key := coordination.DirectPostIdempotencyKey(7, 3, 99, intakeCanonical)
	w, _ := db.Coordination().GetDirectPostCommentWorkflowByIdempotencyKey(ctx, 7, key)
	// Drive it to terminal failure.
	if ok, err := db.Coordination().MarkDirectPostFailed(ctx, 7, w.ID, coordination.DPErrLeadNotObserved, "x"); err != nil || !ok {
		t.Fatalf("mark failed: ok=%v err=%v", ok, err)
	}
	// Re-prompt → reset to a fresh import_queued (NOT stuck failed).
	if _, err := intake.request(ctx, in); err != nil {
		t.Fatal(err)
	}
	again, _ := db.Coordination().GetDirectPostCommentWorkflowByIdempotencyKey(ctx, 7, key)
	if again.Status != coordination.DPStatusImportQueued || again.RetryCount != 0 || again.ErrorCode != "" {
		t.Errorf("re-request after failure should re-open the workflow: status=%s n=%d code=%s", again.Status, again.RetryCount, again.ErrorCode)
	}
}
