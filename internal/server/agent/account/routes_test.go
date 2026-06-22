package account

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/server/testsupport"
)

// TestRegisterRoutes_ReadinessPath proves the extraction preserved the route
// surface: RegisterRoutes mounts GET /accounts/readiness, the handler returns
// 401 without an org context and a {"accounts":[...]} body with one.
func TestRegisterRoutes_ReadinessPath(t *testing.T) {
	db := testsupport.NewTestStore(t, "account_routes")

	t.Run("missing org context returns 401", func(t *testing.T) {
		app := fiber.New()
		RegisterRoutes(app, Deps{DB: db})
		resp, err := app.Test(httptest.NewRequest("GET", "/accounts/readiness", nil))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != 401 {
			t.Fatalf("no-org GET /accounts/readiness = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("with org context returns the accounts matrix", func(t *testing.T) {
		app := fiber.New()
		app.Use(func(c *fiber.Ctx) error {
			c.Locals("org_id", int64(5))
			c.Locals("user_id", int64(1))
			c.Locals("user_role", "admin")
			return c.Next()
		})
		RegisterRoutes(app, Deps{DB: db})
		resp, err := app.Test(httptest.NewRequest("GET", "/accounts/readiness", nil))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("GET /accounts/readiness = %d, want 200", resp.StatusCode)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var out struct {
			Accounts []map[string]any `json:"accounts"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode: %v: %s", err, raw)
		}
	})
}
