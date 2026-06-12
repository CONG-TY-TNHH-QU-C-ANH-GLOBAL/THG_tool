package workspace

import (
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

// Admin pause/resume endpoints (PR-2b): org-scoped, flag round-trips,
// foreign-org ids 404. Route-level adminOnly gating is exercised by the
// router wiring; here we exercise the handler contract.
func TestAccountPauseResumeEndpoints(t *testing.T) {
	dst := storetest.CopyTemplate(t, bootstrapStore, "account_pause_api.db")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Staff FB",
		AssignedUserID: 8, Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	h := &Handler{db: db}
	app := fiber.New()
	withLocals := func(orgID int64) fiber.Handler {
		return func(c *fiber.Ctx) error {
			c.Locals("org_id", orgID)
			c.Locals("user_id", int64(1))
			c.Locals("user_role", "admin")
			return c.Next()
		}
	}
	app.Put("/accounts/:id/pause", withLocals(1), h.pauseAccountAssignment)
	app.Put("/accounts/:id/resume", withLocals(1), h.resumeAccountAssignment)
	app.Put("/foreign/accounts/:id/pause", withLocals(2), h.pauseAccountAssignment)

	do := func(path string) int {
		resp, err := app.Test(httptest.NewRequest("PUT", path, nil))
		if err != nil {
			t.Fatalf("request %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if code := do("/accounts/" + itoa(accID) + "/pause"); code != 200 {
		t.Fatalf("pause status = %d, want 200", code)
	}
	if paused, _ := db.Identities().AccountAssignmentPaused(accID); !paused {
		t.Fatalf("flag not set after pause")
	}

	// Foreign org must 404 and must not flip the flag back.
	if code := do("/foreign/accounts/" + itoa(accID) + "/pause"); code != 404 {
		t.Fatalf("foreign-org pause status = %d, want 404", code)
	}

	if code := do("/accounts/" + itoa(accID) + "/resume"); code != 200 {
		t.Fatalf("resume status = %d, want 200", code)
	}
	if paused, _ := db.Identities().AccountAssignmentPaused(accID); paused {
		t.Fatalf("flag still set after resume")
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
