package agent

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
	"github.com/thg/scraper/internal/store/identities"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapPairingVerifyStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

// The facebook-status endpoint verifies ONE pairing session and is bound to
// its creator: other members (admin included) get 403, other workspaces 404,
// and the verdict comes from the exact paired connector only.
func TestGetPairingFacebookStatus_ScopeAndVerdict(t *testing.T) {
	dst := storetest.CopyTemplate(t, bootstrapPairingVerifyStore, "pairing_verify.db")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgID, creatorID, otherID = int64(1), int64(8), int64(9)

	pair, err := db.Connectors().CreateConnectorPairingCode("dev", creatorID, orgID, 0, time.Minute)
	if err != nil {
		t.Fatalf("create code: %v", err)
	}
	if _, err := db.Connectors().ClaimConnectorPairingCode(pair.Code, connectors.AgentPresence{
		BrowserProfileID: "profile-verify",
		StreamStatus:     "facebook_logged_in",
		FBUserID:         "fb-777",
	}); err != nil {
		t.Fatalf("claim: %v", err)
	}

	h := &LocalConnectorHandler{db: db}
	get := func(viewerOrg, viewerID int64) (int, string) {
		app := fiber.New()
		app.Get("/connectors/pairing/:id/facebook-status", func(c *fiber.Ctx) error {
			c.Locals("org_id", viewerOrg)
			c.Locals("user_id", viewerID)
			return h.getPairingFacebookStatus(c)
		})
		req := httptest.NewRequest("GET", "/connectors/pairing/"+itoa64(pair.ID)+"/facebook-status", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var body struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(raw, &body)
		return resp.StatusCode, body.Status
	}

	// Another member's heartbeat/identity can never verify this session.
	if code, _ := get(orgID, otherID); code != 403 {
		t.Errorf("other member status = %d, want 403", code)
	}
	// Owner binding is strict — there is no admin bypass for verification.
	if code, _ := get(orgID, int64(1)); code != 403 {
		t.Errorf("admin (non-creator) status = %d, want 403", code)
	}
	// Session is invisible outside its workspace.
	if code, _ := get(int64(2), creatorID); code != 404 {
		t.Errorf("cross-workspace status = %d, want 404", code)
	}
	// Creator sees the live verdict from the exact paired connector.
	if code, status := get(orgID, creatorID); code != 200 || status != string(connectors.PairingStatusDetected) {
		t.Errorf("creator got %d/%q, want 200/detected", code, status)
	}

	// Same fb identity owned by ANOTHER member flips the verdict to conflict.
	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: "Other member FB",
		AssignedUserID: otherID, Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if err := db.Identities().SetAccountFacebookIdentity(accID, "fb-777", "", identities.FacebookIdentityMeta{}); err != nil {
		t.Fatalf("bind fb identity: %v", err)
	}
	if code, status := get(orgID, creatorID); code != 200 || status != string(connectors.PairingStatusConflict) {
		t.Errorf("conflict case got %d/%q, want 200/conflict", code, status)
	}
}
