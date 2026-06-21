package superadmin

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestRegisterRoutes_PrefixAndFounderGate proves the extraction preserved the
// route surface: RegisterRoutes mounts handlers under the exact "/superadmin"
// prefix with the founder-only middleware ahead of each handler in the chain.
// (In production the parent group adds the "/api" prefix, unchanged here.)
func TestRegisterRoutes_PrefixAndFounderGate(t *testing.T) {
	db := newTestStore(t, "superadmin_routes.db")

	t.Run("paths mount under /superadmin and gate runs before handler", func(t *testing.T) {
		gateCalls := 0
		founderOnly := func(c *fiber.Ctx) error {
			gateCalls++
			return c.Next()
		}
		app := fiber.New()
		RegisterRoutes(app, Deps{DB: db}, founderOnly)

		// GET /superadmin/orgs reaches listOrgs (empty store -> 200).
		resp, err := app.Test(httptest.NewRequest("GET", "/superadmin/orgs", nil))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("GET /superadmin/orgs = %d, want 200", resp.StatusCode)
		}
		if gateCalls == 0 {
			t.Fatal("founderOnly middleware was not in the chain for /superadmin/orgs")
		}
	})

	t.Run("founder gate blocks before the handler runs", func(t *testing.T) {
		blockingGate := func(c *fiber.Ctx) error {
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}
		app := fiber.New()
		RegisterRoutes(app, Deps{DB: db}, blockingGate)

		// A blocked POST /superadmin/query must return the gate's 403, proving
		// the middleware runs ahead of superAdminQuery.
		resp, err := app.Test(httptest.NewRequest("POST", "/superadmin/query", nil))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != 403 {
			t.Fatalf("blocked POST /superadmin/query = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("unregistered subpath is 404", func(t *testing.T) {
		app := fiber.New()
		RegisterRoutes(app, Deps{DB: db}, func(c *fiber.Ctx) error { return c.Next() })
		resp, err := app.Test(httptest.NewRequest("GET", "/superadmin/nope", nil))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != 404 {
			t.Fatalf("GET /superadmin/nope = %d, want 404", resp.StatusCode)
		}
	})
}
