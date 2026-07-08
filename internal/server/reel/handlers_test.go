// Transport-level Reel API tests — feature-flag gating, auth/org scope, and
// request validation. These never reach the (Postgres-only) reel store, so
// they run on a SQLite test store and need no POSTGRES_PLATFORM_TEST_DSN.
package reel_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	serverreel "github.com/thg/scraper/internal/server/reel"
	"github.com/thg/scraper/internal/server/testsupport"
	"github.com/thg/scraper/internal/store"
)

// testApp mounts the reel routes on a bare fiber app. When org > 0 a
// middleware injects the authenticated context the real router's auth
// middleware sets (org_id/user_id/user_role); org == 0 leaves it absent to
// simulate a missing auth context.
func testApp(db *store.Store, enabled bool, org, user int64) *fiber.App {
	app := fiber.New()
	if org > 0 {
		app.Use(func(c *fiber.Ctx) error {
			c.Locals("org_id", org)
			c.Locals("user_id", user)
			c.Locals("user_role", "admin")
			return c.Next()
		})
	}
	serverreel.Routes(app, serverreel.Deps{DB: db, Enabled: enabled})
	return app
}

// postStatus drives a POST and returns only the status code.
func postStatus(t *testing.T, app *fiber.App, path, body string) int {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestReelAPI_FeatureDisabled_NotMounted(t *testing.T) {
	db := testsupport.NewTestStore(t, "reel_api_disabled")
	app := testApp(db, false, 5, 1)
	if code := postStatus(t, app, "/reels/", `{"title":"x"}`); code != 404 {
		t.Fatalf("disabled POST /reels = %d, want 404", code)
	}
	if code := postStatus(t, app, "/reels/1/approve", ``); code != 404 {
		t.Fatalf("disabled POST /reels/1/approve = %d, want 404", code)
	}
}

func TestReelAPI_MissingOrg_401(t *testing.T) {
	db := testsupport.NewTestStore(t, "reel_api_noorg")
	app := testApp(db, true, 0, 0) // enabled, but no org-injecting middleware
	if code := postStatus(t, app, "/reels/", `{"title":"x"}`); code != 401 {
		t.Fatalf("no-org POST /reels = %d, want 401", code)
	}
}

func TestReelAPI_InvalidReelID_400(t *testing.T) {
	db := testsupport.NewTestStore(t, "reel_api_badid")
	app := testApp(db, true, 5, 1)
	if code := postStatus(t, app, "/reels/not-a-number/script", ``); code != 400 {
		t.Fatalf("bad reel_id = %d, want 400", code)
	}
}

func TestReelAPI_EmptyTitle_400(t *testing.T) {
	db := testsupport.NewTestStore(t, "reel_api_notitle")
	app := testApp(db, true, 5, 1)
	if code := postStatus(t, app, "/reels/", `{"title":"   "}`); code != 400 {
		t.Fatalf("empty title = %d, want 400", code)
	}
}
