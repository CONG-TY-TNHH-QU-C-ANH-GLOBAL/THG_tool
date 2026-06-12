package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

// FreshOrgClaim (PR-2b): a mutating request whose JWT org claim no
// longer matches users.org_id (e.g. token issued before an invite
// accept moved the user) must be rejected with SESSION_STALE. Reads
// pass untouched; matching claims pass.
func TestFreshOrgClaim_BlocksStaleOrgWrites(t *testing.T) {
	dst := storetest.CopyTemplate(t, bootstrapStore, "fresh_org.db")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	userID, err := db.CreateUser(&models.User{
		OrgID: 2, Email: "moved@example.com", Name: "Moved User",
		PasswordHash: "x", Role: models.UserRole("sales"),
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	newApp := func(claimOrg int64) *fiber.App {
		app := fiber.New()
		inject := func(c *fiber.Ctx) error {
			c.Locals("user_id", userID)
			c.Locals("org_id", claimOrg)
			c.Locals("user_role", "sales")
			return c.Next()
		}
		app.Use(inject, FreshOrgClaim(db))
		ok := func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) }
		app.Get("/read", ok)
		app.Post("/write", ok)
		return app
	}

	status := func(app *fiber.App, method, path string) int {
		resp, err := app.Test(httptest.NewRequest(method, path, nil))
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	stale := newApp(1) // JWT claims org 1, DB says org 2
	if code := status(stale, "POST", "/write"); code != 401 {
		t.Errorf("stale-claim write status = %d, want 401", code)
	}
	if code := status(stale, "GET", "/read"); code != 200 {
		t.Errorf("stale-claim read status = %d, want 200 (reads untouched)", code)
	}

	fresh := newApp(2) // claim matches DB
	if code := status(fresh, "POST", "/write"); code != 200 {
		t.Errorf("fresh-claim write status = %d, want 200", code)
	}
}
