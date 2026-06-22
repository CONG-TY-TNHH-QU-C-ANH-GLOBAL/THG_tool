package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Characterization-test support for processConnectorCrawlResult (crawl_ingest.go).
// Each test gets its own fresh store (testsupport.NewTestStore registers t.Cleanup
// to close it), so no DB state is shared across tests — assertions still filter by
// org/task/account id. The Handler is built white-box with only the two fields the
// no-lead paths touch (db + the existing notifier func seam); aiClass/tgEvents stay
// nil because these paths create no lead and so never run the OnLeadCreated closure.
// No production signature or seam is added.

// recordingNotifier returns a notifier func plus a pointer to the messages it
// captured. NotifyCrawlSummary/NotifyCrawlFailure call the notifier synchronously,
// so reads after app.Test returns are race-free and deterministic (no sleeps).
func recordingNotifier() (func(string), *[]string) {
	var msgs []string
	return func(s string) { msgs = append(msgs, s) }, &msgs
}

// newCrawlApp mounts agentConnectorCrawlResult with the agent_id / agent_org_id
// locals the real auth middleware would set, so tests drive the exact HTTP edge.
func newCrawlApp(h *Handler, agentID, orgID int64) *fiber.App {
	app := fiber.New()
	app.Post("/crawl-result", func(c *fiber.Ctx) error {
		c.Locals("agent_id", agentID)
		c.Locals("agent_org_id", orgID)
		return h.agentConnectorCrawlResult(c)
	})
	return app
}

// postCrawl sends a RAW JSON body through the handler's BodyParser decode path and
// returns the status + decoded response map. app.Test(req, -1) disables the test
// timeout so the synchronous ingest completes — it is not a timing wait.
func postCrawl(t *testing.T, app *fiber.App, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/crawl-result", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// seedCrawlAccount inserts an active Facebook account in orgID and returns its id.
func seedCrawlAccount(t *testing.T, db *store.Store, orgID int64) int64 {
	t.Helper()
	id, err := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: "crawl-acc", Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("AddAccount(org=%d): %v", orgID, err)
	}
	return id
}

// seedOwningConnector inserts an ONLINE connector assigned to accID and returns its
// id (the agent_id). This makes ConnectorOwnsAccountStream(orgID, id, accID) true.
func seedOwningConnector(t *testing.T, db *store.Store, orgID, accID int64) int64 {
	t.Helper()
	res, err := db.DB().Exec(
		`INSERT INTO agent_tokens
			(org_id, name, created_by, token_hash, kind, transport, assigned_account_id,
			 stream_status, version, active, last_seen, created_at)
		 VALUES (?, 'ext', 1, ?, 'extension_connector', 'chrome_extension', ?,
		        'facebook_logged_in', '9.9.9', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		orgID, fmt.Sprintf("hash-%d-%d", orgID, accID), accID,
	)
	if err != nil {
		t.Fatalf("seed connector: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// crawlAppStore opens an AppStore for reading task/lead side effects.
func crawlAppStore(t *testing.T, db *store.Store) *store.AppStore {
	t.Helper()
	as, err := store.NewAppStore(db)
	if err != nil {
		t.Fatalf("NewAppStore: %v", err)
	}
	return as
}

// requireNoTask asserts no app_task row exists for taskID (a rejected ingest must
// not have created/started a task).
func requireNoTask(t *testing.T, as *store.AppStore, taskID string) {
	t.Helper()
	if task, err := as.GetTask(context.Background(), taskID); err == nil {
		t.Fatalf("expected no task for %q, but found one: %+v", taskID, task)
	}
}

// requireLeadCount asserts the number of stored leads for orgID (filters by org —
// never a global table count).
func requireLeadCount(t *testing.T, as *store.AppStore, orgID int64, want int) {
	t.Helper()
	leads, err := as.ListLeads(context.Background(), orgID, "", "", 0, 1000, 0)
	if err != nil {
		t.Fatalf("ListLeads(org=%d): %v", orgID, err)
	}
	if len(leads) != want {
		t.Fatalf("lead count for org %d = %d, want %d", orgID, len(leads), want)
	}
}
