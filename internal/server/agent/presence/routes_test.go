package presence

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/server/testsupport"
	"github.com/thg/scraper/internal/store"
)

func passThroughGate(c *fiber.Ctx) error { return c.Next() }

func blockForbiddenGate(c *fiber.Ctx) error {
	return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
}

// newPresenceRoutesApp builds a fiber app whose middleware injects org locals for
// the given role, then mounts the presence boards via RegisterRoutes with the
// supplied admin gate — mirroring how agent.DashboardRoutes wires them.
func newPresenceRoutesApp(db *store.Store, role string, adminOnly fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("org_id", int64(1))
		c.Locals("user_id", int64(1))
		c.Locals("user_role", role)
		return c.Next()
	})
	RegisterRoutes(app, Deps{DB: db}, adminOnly)
	return app
}

// requirePresenceRouteStatus asserts GET path returns the expected status.
func requirePresenceRouteStatus(t *testing.T, app *fiber.App, path string, want int) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", path, nil))
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	if resp.StatusCode != want {
		t.Fatalf("GET %s = %d, want %d", path, resp.StatusCode, want)
	}
}

// TestRegisterRoutes_PathsAndAdminGate proves the extraction preserved the route
// surface: RegisterRoutes mounts /connectors/status (tenant) and
// /admin/connectors/overview (adminOnly) on the given group, with the admin gate
// on the overview only — exactly as they were inside agent.DashboardRoutes.
func TestRegisterRoutes_PathsAndAdminGate(t *testing.T) {
	db := testsupport.NewTestStore(t, "presence_routes.db")

	t.Run("both boards mount under their exact paths", func(t *testing.T) {
		app := newPresenceRoutesApp(db, "admin", passThroughGate)
		requirePresenceRouteStatus(t, app, "/connectors/status", 200)
		requirePresenceRouteStatus(t, app, "/admin/connectors/overview", 200)
	})

	t.Run("adminOnly gates the overview but not the status board", func(t *testing.T) {
		app := newPresenceRoutesApp(db, "sales", blockForbiddenGate)
		requirePresenceRouteStatus(t, app, "/admin/connectors/overview", 403)
		requirePresenceRouteStatus(t, app, "/connectors/status", 200)
	})
}
