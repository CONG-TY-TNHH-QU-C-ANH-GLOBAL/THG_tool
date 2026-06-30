package outbox

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Characterization-test support for finalizeOutbound (outbox_agent.go) via the
// real /agent/outbox/:id/{sent,failed} HTTP handlers. Each test uses its own
// fresh testsupport store (t.Cleanup-closed); the Handler is built white-box with
// only db + the existing notifier func seam. tgEvents stays nil — finalizeOutbound
// guards it (`if h.tgEvents != nil`). NotifyOutboundStatus[Detail] call the
// notifier synchronously, so reads after app.Test return are race-free.
// No production signature or seam is added. Reuses recordingNotifier /
// seedCrawlAccount from crawl_ingest_characterization_test.go.

// newOutboxApp mounts the terminal-callback handlers with the agent_org_id local
// the real agentAuth middleware would set, so tests drive the exact HTTP edge.
func newOutboxApp(h *Handler, orgID int64) *fiber.App {
	app := fiber.New()
	mw := func(c *fiber.Ctx) error {
		c.Locals("agent_org_id", orgID)
		return c.Next()
	}
	app.Post("/agent/outbox/:id/sent", mw, h.agentOutboxSent)
	app.Post("/agent/outbox/:id/failed", mw, h.agentOutboxFailed)
	return app
}

// seedClaimedOutbound inserts a planned comment outbound for the account, then
// CLAIMS it through the production path — mirroring agentGetOutbox — leaving the
// row in `executing` with a fresh execution_id the terminal callback must echo.
func seedClaimedOutbound(t *testing.T, db *store.Store, orgID, accountID int64) (int64, string) {
	t.Helper()
	id, err := db.Outbound().Insert(&models.OutboundMessage{
		OrgID: orgID, Type: "comment", Platform: models.PlatformFacebook, AccountID: accountID,
		TargetURL: "https://www.facebook.com/groups/1/posts/100/", Content: "đặt một bình luận thử",
	})
	if err != nil {
		t.Fatalf("insert outbound: %v", err)
	}
	claim, err := db.Outbound().Claim(orgID, id, "worker-1", time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim outbound: %v (claim=%v)", err, claim)
	}
	return id, claim.ExecutionID
}

// postOutboxCallback posts a RAW JSON body to /agent/outbox/<id>/<kind> through
// the handler's BodyParser decode path and returns status + decoded response.
func postOutboxCallback(t *testing.T, app *fiber.App, kind string, id int64, body string) (int, map[string]any) {
	t.Helper()
	path := "/agent/outbox/" + itoa64(id) + "/" + kind
	req := httptest.NewRequest("POST", path, bytes.NewReader([]byte(body)))
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

// requireOutboundState asserts the outbound row's execution_state.
func requireOutboundState(t *testing.T, db *store.Store, orgID, id int64, want models.ExecutionState) *models.OutboundMessage {
	t.Helper()
	msg, err := db.Outbound().Get(orgID, id)
	if err != nil || msg == nil {
		t.Fatalf("get outbound %d: %v", id, err)
	}
	if msg.ExecutionState != want {
		t.Fatalf("execution_state = %q, want %q", msg.ExecutionState, want)
	}
	return msg
}
