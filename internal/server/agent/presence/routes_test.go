package presence

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/server/testsupport"
)

// TestRegisterRoutes_PathsAndAdminGate proves the extraction preserved the route
// surface: RegisterRoutes mounts /connectors/status (tenant) and
// /admin/connectors/overview (adminOnly) on the given group, with the admin gate
// on the overview only — exactly as they were inside agent.DashboardRoutes.
func TestRegisterRoutes_PathsAndAdminGate(t *testing.T) {
	db := testsupport.NewTestStore(t, "presence_routes.db")
	withOrgLocals := func(role string) fiber.Handler {
		return func(c *fiber.Ctx) error {
			c.Locals("org_id", int64(1))
			c.Locals("user_id", int64(1))
			c.Locals("user_role", role)
			return c.Next()
		}
	}

	t.Run("both boards mount under their exact paths", func(t *testing.T) {
		app := fiber.New()
		app.Use(withOrgLocals("admin"))
		RegisterRoutes(app, Deps{DB: db}, func(c *fiber.Ctx) error { return c.Next() })
		for _, p := range []string{"/connectors/status", "/admin/connectors/overview"} {
			resp, err := app.Test(httptest.NewRequest("GET", p, nil))
			if err != nil {
				t.Fatalf("GET %s: %v", p, err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("GET %s = %d, want 200", p, resp.StatusCode)
			}
		}
	})

	t.Run("adminOnly gates the overview but not the status board", func(t *testing.T) {
		app := fiber.New()
		app.Use(withOrgLocals("sales"))
		blocked := func(c *fiber.Ctx) error { return c.Status(403).JSON(fiber.Map{"error": "forbidden"}) }
		RegisterRoutes(app, Deps{DB: db}, blocked)

		over, err := app.Test(httptest.NewRequest("GET", "/admin/connectors/overview", nil))
		if err != nil {
			t.Fatalf("overview request: %v", err)
		}
		if over.StatusCode != 403 {
			t.Fatalf("overview behind adminOnly = %d, want 403", over.StatusCode)
		}
		status, err := app.Test(httptest.NewRequest("GET", "/connectors/status", nil))
		if err != nil {
			t.Fatalf("status request: %v", err)
		}
		if status.StatusCode != 200 {
			t.Fatalf("status board (no admin gate) = %d, want 200", status.StatusCode)
		}
	})
}
