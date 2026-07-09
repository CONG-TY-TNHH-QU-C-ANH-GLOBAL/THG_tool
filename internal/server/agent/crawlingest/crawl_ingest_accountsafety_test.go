package crawlingest

import (
	"testing"
	"time"

	"github.com/thg/scraper/internal/server/testsupport"
	"github.com/thg/scraper/internal/session/accountsafety"
)

// PR-C4 result feedback: the crawl-result ingest reports terminal results to the
// Account Safety Coordinator so the machine slot the scheduler consumed frees the
// moment the result arrives — never via the stale-timeout fallback.

// newRunningSafetyEnv seeds an owned account + connector and a coordinator whose
// single machine slot is consumed by that account — exactly the scheduler's state
// while the extension crawl is in flight.
func newRunningSafetyEnv(t *testing.T, name string) (h *Handler, accID, connID int64, coord *accountsafety.Coordinator) {
	t.Helper()
	db := testsupport.NewTestStore(t, name)
	const orgID = int64(1)
	accID = seedCrawlAccount(t, db, orgID)
	connID = seedOwningConnector(t, db, orgID, accID)
	coord = accountsafety.NewCoordinator(accountsafety.DefaultConfig(), 15*time.Minute)
	coord.MarkRunning(accID, time.Now().UTC())
	notify, _ := recordingNotifier()
	h = &Handler{db: db, notifier: notify, accountSafety: coord}
	return h, accID, connID, coord
}

// safetyCrawlBody builds the minimal crawl-result payload for an exit_reason.
func safetyCrawlBody(accountID int64, taskID, exitReason string) string {
	return `{"task_id":"` + taskID + `","account_id":` + itoa64(accountID) + `,"exit_reason":"` + exitReason + `","items":[]}`
}

// postSafetyCrawlResult posts body as the env's connector and asserts the exact
// HTTP outcome. wantStatus "" skips the JSON status check (error responses).
func postSafetyCrawlResult(t *testing.T, h *Handler, connID int64, body string, wantCode int, wantStatus string) {
	t.Helper()
	app := newCrawlApp(h, connID, 1)
	code, out := postCrawl(t, app, body)
	if code != wantCode || (wantStatus != "" && out["status"] != wantStatus) {
		t.Fatalf("result = %d %v, want %d %s", code, out, wantCode, wantStatus)
	}
}

// assertSlotFreeAndStatus asserts the PR-C4 post-condition at the CURRENT
// instant: the machine slot is already free (no stale timeout involved) and the
// account landed on exactly the expected safety status.
func assertSlotFreeAndStatus(t *testing.T, coord *accountsafety.Coordinator, accID int64, want accountsafety.Status) {
	t.Helper()
	now := time.Now().UTC()
	if got := coord.FreeSlots(now); got != 1 {
		t.Errorf("free slots after ingest = %d, want 1 immediately (no stale timeout)", got)
	}
	if got := coord.Snapshot(now).Accounts[accID]; got != want {
		t.Errorf("account status = %s, want %s", got, want)
	}
}

// A clean result frees the slot immediately and returns the account to ready.
func TestCrawlResultCompletedFreesSlotImmediately(t *testing.T) {
	h, accID, connID, coord := newRunningSafetyEnv(t, "crawl_safety_free")
	if coord.FreeSlots(time.Now().UTC()) != 0 {
		t.Fatal("precondition: the running crawl must consume the only slot")
	}

	postSafetyCrawlResult(t, h, connID, safetyCrawlBody(accID, "t-safety-free", "completed"), 200, "stored")
	assertSlotFreeAndStatus(t, coord, accID, accountsafety.StatusReady)
}

// checkpoint_suspected frees the slot but parks the account: not eligible for
// future scheduler ticks until an operator resolves it.
func TestCrawlResultCheckpointParksAccount(t *testing.T) {
	h, accID, connID, coord := newRunningSafetyEnv(t, "crawl_safety_park")

	postSafetyCrawlResult(t, h, connID, safetyCrawlBody(accID, "t-safety-park", "checkpoint_suspected"), 200, "stored")
	assertSlotFreeAndStatus(t, coord, accID, accountsafety.StatusCheckpointRequired)
	if coord.IsAccountEligible(accID, time.Now().UTC().Add(1000*time.Hour)) {
		t.Error("parked account must stay ineligible until operator resolution")
	}
}

// An extension-reported failed crawl is also terminal: the slot frees and the
// empty exit_reason follows the clean policy default.
func TestCrawlResultFailedStatusFreesSlot(t *testing.T) {
	h, accID, connID, coord := newRunningSafetyEnv(t, "crawl_safety_failed")

	body := `{"task_id":"t-safety-fail","account_id":` + itoa64(accID) + `,"status":"failed","error":"chrome_crash","items":[]}`
	postSafetyCrawlResult(t, h, connID, body, 200, "failed")
	assertSlotFreeAndStatus(t, coord, accID, accountsafety.StatusReady)
}

// Ownership gates run BEFORE the coordinator update: a result for an account the
// org does not own is rejected and must not touch slot state.
func TestCrawlResultForbiddenDoesNotTouchCoordinator(t *testing.T) {
	h, accID, connID, coord := newRunningSafetyEnv(t, "crawl_safety_forbidden")

	postSafetyCrawlResult(t, h, connID, safetyCrawlBody(999999, "t-safety-forbidden", "completed"), 403, "")
	now := time.Now().UTC()
	if got := coord.FreeSlots(now); got != 0 {
		t.Errorf("free slots = %d, want 0 — a rejected result must not free the slot", got)
	}
	if got := coord.Snapshot(now).Accounts[accID]; got != accountsafety.StatusRunning {
		t.Errorf("running account status = %s, want running (untouched)", got)
	}
}
