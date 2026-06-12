package agent

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

// Remote input commands drive the OWNER's real Chrome. Device privacy
// (PR-M5 / PR-2b): only the owning member may queue input; admin may
// drive unassigned org accounts only — NEVER a staff member's device.
func TestCreateConnectorInputCommand_OwnershipGate(t *testing.T) {
	dst := storetest.CopyTemplate(t, bootstrapInputRBACStore, "input_rbac.db")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	staffAcc, err := db.Identities().AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Staff8 FB",
		AssignedUserID: 8, Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("seed staff account: %v", err)
	}
	unassignedAcc, err := db.Identities().AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Unassigned FB",
		Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("seed unassigned account: %v", err)
	}

	h := &LocalConnectorHandler{db: db}
	post := func(userID int64, role string, accountID int64) int {
		app := fiber.New()
		app.Post("/connectors/input", func(c *fiber.Ctx) error {
			c.Locals("org_id", int64(1))
			c.Locals("user_id", userID)
			c.Locals("user_role", role)
			return h.createConnectorInputCommand(c)
		})
		body := `{"account_id":` + itoa64(accountID) + `,"type":"click","payload":{}}`
		req := httptest.NewRequest("POST", "/connectors/input", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	// Another sales member must not drive staff 8's device.
	if code := post(9, "sales", staffAcc); code != 403 {
		t.Errorf("sales->colleague device status = %d, want 403", code)
	}
	// Admin must not drive a staff member's device either (no remote control of staff Chrome).
	if code := post(1, "admin", staffAcc); code != 403 {
		t.Errorf("admin->staff device status = %d, want 403", code)
	}
	// Owner passes the ownership gate; with no live stream the handler
	// reports 409 (stream not ready) — i.e. the gate is no longer the blocker.
	if code := post(8, "sales", staffAcc); code != 409 {
		t.Errorf("owner status = %d, want 409 (passes gate, no stream)", code)
	}
	// Admin may drive unassigned org-owned accounts.
	if code := post(1, "admin", unassignedAcc); code != 409 {
		t.Errorf("admin->unassigned status = %d, want 409 (passes gate, no stream)", code)
	}
}

func bootstrapInputRBACStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func itoa64(v int64) string {
	return strconv.FormatInt(v, 10)
}
