package crawlingest

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/server/testsupport"
)

// TestProcessConnectorCrawlResult_HappyEmptyItems pins the happy-path skeleton:
// valid ownership + an empty items list → the task is created, started and
// completed (0 fetched / 0 inserted), no leads are stored, the summary notifier
// fires once, and the handler returns the {stored,...} JSON shape.
func TestProcessConnectorCrawlResult_HappyEmptyItems(t *testing.T) {
	db := testsupport.NewTestStore(t, "crawl_happy")
	const orgID = int64(1)
	accID := seedCrawlAccount(t, db, orgID)
	connID := seedOwningConnector(t, db, orgID, accID)

	notify, notes := recordingNotifier()
	h := &Handler{db: db, notifier: notify}
	app := newCrawlApp(h, connID, orgID)

	body := `{"task_id":"t-happy","account_id":` + itoa64(accID) + `,"intent":"facebook_crawl","exit_reason":"end_of_feed","items":[]}`
	code, out := postCrawl(t, app, body)

	if code != 200 || out["status"] != "stored" || out["task_id"] != "t-happy" {
		t.Fatalf("happy result = %d %v, want 200 stored t-happy", code, out)
	}
	if out["fetched"] != float64(0) || out["inserted"] != float64(0) {
		t.Fatalf("counters = fetched %v inserted %v, want 0/0", out["fetched"], out["inserted"])
	}
	as := crawlAppStore(t, db)
	task, err := as.GetTask(context.Background(), "t-happy")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != "completed" || task.OrgID != orgID || task.TotalFetched != 0 || task.TotalReturned != 0 {
		t.Fatalf("task = %+v, want completed org=%d 0/0", task, orgID)
	}
	requireLeadCount(t, as, orgID, 0)
	if len(*notes) != 1 || !strings.Contains((*notes)[0], "t-happy") {
		t.Fatalf("summary notifier = %v, want exactly 1 mentioning the task", *notes)
	}
}

// TestProcessConnectorCrawlResult_FailedStatus pins the extension-reported-failure
// path: the task is marked failed with the verbatim error, the failure notifier
// fires once, no lead is stored, and the handler returns {failed,error}.
func TestProcessConnectorCrawlResult_FailedStatus(t *testing.T) {
	db := testsupport.NewTestStore(t, "crawl_failed")
	const orgID = int64(1)
	accID := seedCrawlAccount(t, db, orgID)
	connID := seedOwningConnector(t, db, orgID, accID)

	notify, notes := recordingNotifier()
	h := &Handler{db: db, notifier: notify}
	app := newCrawlApp(h, connID, orgID)

	body := `{"task_id":"t-fail","account_id":` + itoa64(accID) + `,"status":"failed","error":"chrome_crash","items":[]}`
	code, out := postCrawl(t, app, body)

	if code != 200 || out["status"] != "failed" || out["error"] != "chrome_crash" {
		t.Fatalf("failed result = %d %v, want 200 failed chrome_crash", code, out)
	}
	as := crawlAppStore(t, db)
	task, err := as.GetTask(context.Background(), "t-fail")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != "failed" || task.Error != "chrome_crash" {
		t.Fatalf("task = %+v, want failed error=chrome_crash", task)
	}
	requireLeadCount(t, as, orgID, 0)
	if len(*notes) != 1 || !strings.Contains((*notes)[0], "t-fail") {
		t.Fatalf("failure notifier = %v, want exactly 1 mentioning the task", *notes)
	}
}

// TestProcessConnectorCrawlResult_ForbiddenForeignAccount pins tenant isolation:
// a crawl result for an account in another org is rejected 403 before any task is
// created, and the notifier is never called.
func TestProcessConnectorCrawlResult_ForbiddenForeignAccount(t *testing.T) {
	db := testsupport.NewTestStore(t, "crawl_forbid_acct")
	const callerOrg, otherOrg = int64(1), int64(2)
	foreignAcc := seedCrawlAccount(t, db, otherOrg)

	notify, notes := recordingNotifier()
	h := &Handler{db: db, notifier: notify}
	app := newCrawlApp(h, 999999, callerOrg) // caller is org 1, account lives in org 2

	body := `{"task_id":"t-foreign","account_id":` + itoa64(foreignAcc) + `,"items":[]}`
	code, out := postCrawl(t, app, body)

	if code != 403 || out["error"] != "account does not belong to this organization" {
		t.Fatalf("foreign-account result = %d %v, want 403 forbidden-account", code, out)
	}
	as := crawlAppStore(t, db)
	requireNoTask(t, as, "t-foreign")
	requireLeadCount(t, as, callerOrg, 0)
	requireLeadCount(t, as, otherOrg, 0)
	if len(*notes) != 0 {
		t.Fatalf("notifier must not fire on rejected ingest, got %v", *notes)
	}
}

// TestProcessConnectorCrawlResult_ForbiddenStream pins the stream-ownership gate:
// the account belongs to the org but no connector owns its stream → 403, no task,
// no notification.
func TestProcessConnectorCrawlResult_ForbiddenStream(t *testing.T) {
	db := testsupport.NewTestStore(t, "crawl_forbid_stream")
	const orgID = int64(1)
	accID := seedCrawlAccount(t, db, orgID) // account in org, but NO owning connector seeded

	notify, notes := recordingNotifier()
	h := &Handler{db: db, notifier: notify}
	app := newCrawlApp(h, 999999, orgID) // agent_id does not match any connector

	body := `{"task_id":"t-stream","account_id":` + itoa64(accID) + `,"items":[]}`
	code, out := postCrawl(t, app, body)

	if code != 403 || out["error"] != "connector does not own this account stream" {
		t.Fatalf("forbidden-stream result = %d %v, want 403 forbidden-stream", code, out)
	}
	as := crawlAppStore(t, db)
	requireNoTask(t, as, "t-stream")
	if len(*notes) != 0 {
		t.Fatalf("notifier must not fire on rejected ingest, got %v", *notes)
	}
}

// TestProcessConnectorCrawlResult_MalformedJSON pins the decode-failure edge:
// invalid JSON is rejected 400 by the handler before the processor runs, so no
// task is created and no notification fires.
func TestProcessConnectorCrawlResult_MalformedJSON(t *testing.T) {
	db := testsupport.NewTestStore(t, "crawl_malformed")
	const orgID = int64(1)

	notify, notes := recordingNotifier()
	h := &Handler{db: db, notifier: notify}
	app := newCrawlApp(h, 1, orgID)

	code, out := postCrawl(t, app, `{"task_id":"t-bad","account_id":`) // truncated, invalid JSON
	if code != 400 || out["error"] != "invalid body" {
		t.Fatalf("malformed result = %d %v, want 400 invalid body", code, out)
	}
	as := crawlAppStore(t, db)
	requireNoTask(t, as, "t-bad")
	if len(*notes) != 0 {
		t.Fatalf("notifier must not fire on malformed body, got %v", *notes)
	}
}
