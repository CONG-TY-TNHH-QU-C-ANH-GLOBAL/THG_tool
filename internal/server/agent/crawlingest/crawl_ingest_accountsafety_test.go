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

// A clean result frees the slot immediately and returns the account to ready.
func TestCrawlResultCompletedFreesSlotImmediately(t *testing.T) {
	h, accID, connID, coord := newRunningSafetyEnv(t, "crawl_safety_free")
	app := newCrawlApp(h, connID, 1)
	if coord.FreeSlots(time.Now().UTC()) != 0 {
		t.Fatal("precondition: the running crawl must consume the only slot")
	}

	body := `{"task_id":"t-safety-free","account_id":` + itoa64(accID) + `,"exit_reason":"completed","items":[]}`
	code, out := postCrawl(t, app, body)
	if code != 200 || out["status"] != "stored" {
		t.Fatalf("result = %d %v, want 200 stored", code, out)
	}
	now := time.Now().UTC()
	if got := coord.FreeSlots(now); got != 1 {
		t.Errorf("free slots after ingest = %d, want 1 immediately (no stale timeout)", got)
	}
	if got := coord.Snapshot(now).Accounts[accID]; got != accountsafety.StatusReady {
		t.Errorf("account status = %s, want ready", got)
	}
}

// checkpoint_suspected frees the slot but parks the account: not eligible for
// future scheduler ticks until an operator resolves it.
func TestCrawlResultCheckpointParksAccount(t *testing.T) {
	h, accID, connID, coord := newRunningSafetyEnv(t, "crawl_safety_park")
	app := newCrawlApp(h, connID, 1)

	body := `{"task_id":"t-safety-park","account_id":` + itoa64(accID) + `,"exit_reason":"checkpoint_suspected","items":[]}`
	code, out := postCrawl(t, app, body)
	if code != 200 || out["status"] != "stored" {
		t.Fatalf("result = %d %v, want 200 stored", code, out)
	}
	now := time.Now().UTC()
	if got := coord.FreeSlots(now); got != 1 {
		t.Errorf("free slots = %d, want 1 (slot freed for other accounts)", got)
	}
	if got := coord.Snapshot(now).Accounts[accID]; got != accountsafety.StatusCheckpointRequired {
		t.Errorf("account status = %s, want checkpoint_required", got)
	}
	if coord.IsAccountEligible(accID, now.Add(1000*time.Hour)) {
		t.Error("parked account must stay ineligible until operator resolution")
	}
}

// An extension-reported failed crawl is also terminal: the slot frees and the
// empty exit_reason follows the clean policy default.
func TestCrawlResultFailedStatusFreesSlot(t *testing.T) {
	h, accID, connID, coord := newRunningSafetyEnv(t, "crawl_safety_failed")
	app := newCrawlApp(h, connID, 1)

	body := `{"task_id":"t-safety-fail","account_id":` + itoa64(accID) + `,"status":"failed","error":"chrome_crash","items":[]}`
	code, out := postCrawl(t, app, body)
	if code != 200 || out["status"] != "failed" {
		t.Fatalf("result = %d %v, want 200 failed", code, out)
	}
	now := time.Now().UTC()
	if got := coord.FreeSlots(now); got != 1 {
		t.Errorf("free slots after failed crawl = %d, want 1", got)
	}
	if got := coord.Snapshot(now).Accounts[accID]; got != accountsafety.StatusReady {
		t.Errorf("account status = %s, want ready (empty exit_reason = policy default)", got)
	}
}

// Ownership gates run BEFORE the coordinator update: a result for an account the
// org does not own is rejected and must not touch slot state.
func TestCrawlResultForbiddenDoesNotTouchCoordinator(t *testing.T) {
	h, accID, connID, coord := newRunningSafetyEnv(t, "crawl_safety_forbidden")
	app := newCrawlApp(h, connID, 1)

	body := `{"task_id":"t-safety-forbidden","account_id":999999,"exit_reason":"completed","items":[]}`
	code, _ := postCrawl(t, app, body)
	if code != 403 {
		t.Fatalf("foreign account result = %d, want 403", code)
	}
	now := time.Now().UTC()
	if got := coord.FreeSlots(now); got != 0 {
		t.Errorf("free slots = %d, want 0 — a rejected result must not free the slot", got)
	}
	if got := coord.Snapshot(now).Accounts[accID]; got != accountsafety.StatusRunning {
		t.Errorf("running account status = %s, want running (untouched)", got)
	}
}
