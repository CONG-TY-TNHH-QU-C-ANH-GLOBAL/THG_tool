package agent

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/agent/finalize"
	"github.com/thg/scraper/internal/server/testsupport"
)

// TestFinalizeOutbound_VerifiedSentSuccess pins the FIRST-WIN success path: a
// matching-execution_id /sent callback finalizes the claimed row to `finished`
// with a verification_outcome, returns the {execution_state, verification_outcome,
// attempt_id} body, and fires the summary notifier exactly once.
func TestFinalizeOutbound_VerifiedSentSuccess(t *testing.T) {
	db := testsupport.NewTestStore(t, "finalize_success")
	const orgID = int64(1)
	accID := seedCrawlAccount(t, db, orgID)
	id, execID := seedClaimedOutbound(t, db, orgID, accID)

	notify, notes := recordingNotifier()
	h := &Handler{db: db, notifier: notify, finalize: finalize.NewHandler(finalize.Deps{DB: db, Notifier: notify})}
	app := newOutboxApp(h, orgID)

	// Pre-state: the claim left it executing, not yet terminal.
	requireOutboundState(t, db, orgID, id, models.ExecExecuting)

	code, out := postOutboxCallback(t, app, "sent", id, `{"success":true,"execution_id":"`+execID+`"}`)
	if code != 200 {
		t.Fatalf("sent = %d %v, want 200", code, out)
	}
	if out["execution_state"] != string(models.ExecFinished) {
		t.Fatalf("response execution_state = %v, want finished", out["execution_state"])
	}
	if _, ok := out["attempt_id"]; !ok {
		t.Fatalf("response missing attempt_id: %v", out)
	}
	if _, ok := out["verification_outcome"]; !ok {
		t.Fatalf("response missing verification_outcome: %v", out)
	}

	after := requireOutboundState(t, db, orgID, id, models.ExecFinished)
	if after.VerificationOutcome == "" {
		t.Fatalf("verification_outcome not persisted: %+v", after)
	}
	if len(*notes) != 1 {
		t.Fatalf("notifier = %v, want exactly 1", *notes)
	}
}

// TestFinalizeOutbound_IdempotentReplay pins once-only side effects: replaying
// the same /sent callback returns idempotent:true, does NOT re-notify, and leaves
// the terminal state stable.
func TestFinalizeOutbound_IdempotentReplay(t *testing.T) {
	db := testsupport.NewTestStore(t, "finalize_replay")
	const orgID = int64(1)
	accID := seedCrawlAccount(t, db, orgID)
	id, execID := seedClaimedOutbound(t, db, orgID, accID)

	notify, notes := recordingNotifier()
	h := &Handler{db: db, notifier: notify, finalize: finalize.NewHandler(finalize.Deps{DB: db, Notifier: notify})}
	app := newOutboxApp(h, orgID)
	body := `{"success":true,"execution_id":"` + execID + `"}`

	if code, out := postOutboxCallback(t, app, "sent", id, body); code != 200 {
		t.Fatalf("first sent = %d %v, want 200", code, out)
	}
	notesAfterFirst := len(*notes)

	code, out := postOutboxCallback(t, app, "sent", id, body)
	if code != 200 {
		t.Fatalf("replay sent = %d %v, want 200", code, out)
	}
	if out["idempotent"] != true {
		t.Fatalf("replay must report idempotent:true, got %v", out)
	}
	if len(*notes) != notesAfterFirst {
		t.Fatalf("replay must not re-notify: notes %d -> %d", notesAfterFirst, len(*notes))
	}
	requireOutboundState(t, db, orgID, id, models.ExecFinished) // terminal stays stable
}

// TestFinalizeOutbound_StaleExecutionID pins the stale/mismatch guard: a callback
// carrying a different execution_id is rejected 409 and must NOT terminalize the
// row or fire any notification.
func TestFinalizeOutbound_StaleExecutionID(t *testing.T) {
	db := testsupport.NewTestStore(t, "finalize_stale")
	const orgID = int64(1)
	accID := seedCrawlAccount(t, db, orgID)
	id, _ := seedClaimedOutbound(t, db, orgID, accID) // real execID discarded; send a wrong one

	notify, notes := recordingNotifier()
	h := &Handler{db: db, notifier: notify, finalize: finalize.NewHandler(finalize.Deps{DB: db, Notifier: notify})}
	app := newOutboxApp(h, orgID)

	code, out := postOutboxCallback(t, app, "sent", id, `{"success":true,"execution_id":"stale-mismatch-token"}`)
	if code != 409 {
		t.Fatalf("stale = %d %v, want 409", code, out)
	}
	if out["error"] != "stale execution_id" {
		t.Fatalf("stale error = %v, want \"stale execution_id\"", out["error"])
	}
	// Must not terminalize the still-valid execution, and must not notify.
	requireOutboundState(t, db, orgID, id, models.ExecExecuting)
	if len(*notes) != 0 {
		t.Fatalf("stale callback must not notify, got %v", *notes)
	}
}

// TestFinalizeOutbound_FailedNonSuccess pins the non-success terminal path: a
// /failed callback finalizes the row to `finished` with a NON-verified outcome,
// returns the terminal body, and fires the failure notifier once.
func TestFinalizeOutbound_FailedNonSuccess(t *testing.T) {
	db := testsupport.NewTestStore(t, "finalize_failed")
	const orgID = int64(1)
	accID := seedCrawlAccount(t, db, orgID)
	id, execID := seedClaimedOutbound(t, db, orgID, accID)

	notify, notes := recordingNotifier()
	h := &Handler{db: db, notifier: notify, finalize: finalize.NewHandler(finalize.Deps{DB: db, Notifier: notify})}
	app := newOutboxApp(h, orgID)

	code, out := postOutboxCallback(t, app, "failed", id, `{"success":false,"failure_reason":"blocked","execution_id":"`+execID+`"}`)
	if code != 200 {
		t.Fatalf("failed = %d %v, want 200", code, out)
	}
	if out["execution_state"] != string(models.ExecFinished) {
		t.Fatalf("response execution_state = %v, want finished", out["execution_state"])
	}

	after := requireOutboundState(t, db, orgID, id, models.ExecFinished)
	if models.IsVerifiedSuccess(after.ExecutionState, after.VerificationOutcome) {
		t.Fatalf("a blocked failure must NOT be a verified success: %+v", after)
	}
	if len(*notes) != 1 {
		t.Fatalf("notifier = %v, want exactly 1", *notes)
	}
}

// TestFinalizeOutbound_InvalidID pins the handler-level decode guard: a
// non-numeric :id is rejected 400 before any finalize work, and the notifier
// stays silent.
func TestFinalizeOutbound_InvalidID(t *testing.T) {
	db := testsupport.NewTestStore(t, "finalize_badid")
	notify, notes := recordingNotifier()
	h := &Handler{db: db, notifier: notify, finalize: finalize.NewHandler(finalize.Deps{DB: db, Notifier: notify})}
	app := newOutboxApp(h, 1)

	req := httptest.NewRequest("POST", "/agent/outbox/not-a-number/sent", strings.NewReader(`{"success":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("invalid id status = %d, want 400", resp.StatusCode)
	}
	if len(*notes) != 0 {
		t.Fatalf("invalid id must not notify, got %v", *notes)
	}
}
