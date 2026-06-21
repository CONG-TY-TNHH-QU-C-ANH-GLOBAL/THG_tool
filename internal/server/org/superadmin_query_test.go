package org

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
)

// performSuperadminQueryRequest posts a JSON payload to the founder query
// endpoint and returns the HTTP status plus the decoded response body.
func performSuperadminQueryRequest(t *testing.T, app *fiber.App, payload string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/superadmin/query", bytes.NewReader([]byte(payload)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// requireRawSQLRejected asserts a legacy raw-sql payload is rejected with 400
// and that no result set is returned (the SQL must never be executed).
func requireRawSQLRejected(t *testing.T, app *fiber.App) {
	t.Helper()
	code, body := performSuperadminQueryRequest(t, app, `{"sql":"SELECT password_hash FROM users"}`)
	if code != 400 {
		t.Fatalf("raw sql status = %d, want 400", code)
	}
	if _, ok := body["rows"]; ok {
		t.Fatalf("raw sql must not be executed; got rows in response: %v", body)
	}
}

// requireUnknownReportRejected asserts an unknown report key returns 400.
func requireUnknownReportRejected(t *testing.T, app *fiber.App) {
	t.Helper()
	code, body := performSuperadminQueryRequest(t, app, `{"report":"passwords"}`)
	if code != 400 || body["error"] != "invalid report" {
		t.Fatalf("unknown report = %d %v, want 400 invalid report", code, body)
	}
}

// requireUsersReportResponse asserts the allowlisted users report executes,
// returns the expected non-sensitive columns, never exposes password_hash, and
// contains at least the seeded row.
func requireUsersReportResponse(t *testing.T, app *fiber.App) {
	t.Helper()
	code, body := performSuperadminQueryRequest(t, app, `{"report":"users"}`)
	if code != 200 {
		t.Fatalf("users report = %d %v, want 200", code, body)
	}
	got := reportColumnSet(body)
	for _, want := range []string{"id", "email", "name", "role", "active", "org_id", "created_at"} {
		if !got[want] {
			t.Fatalf("users report columns missing %q; got %v", want, body["columns"])
		}
	}
	if got["password_hash"] {
		t.Fatalf("users report must not expose password_hash")
	}
	if n, _ := body["count"].(float64); n < 1 {
		t.Fatalf("users report count = %v, want >= 1", body["count"])
	}
}

// requireOrganizationsReportResponse asserts the allowlisted organizations
// report executes and returns the columns/rows/count response shape.
func requireOrganizationsReportResponse(t *testing.T, app *fiber.App) {
	t.Helper()
	code, body := performSuperadminQueryRequest(t, app, `{"report":"organizations"}`)
	if code != 200 {
		t.Fatalf("organizations report = %d %v, want 200", code, body)
	}
	if _, ok := body["columns"]; !ok {
		t.Fatalf("organizations report missing columns: %v", body)
	}
}

// reportColumnSet collects the response "columns" array into a lookup set.
func reportColumnSet(body map[string]any) map[string]bool {
	cols, _ := body["columns"].([]any)
	got := make(map[string]bool, len(cols))
	for _, col := range cols {
		if name, ok := col.(string); ok {
			got[name] = true
		}
	}
	return got
}

// TestSuperAdminQuery_AllowlistOnly pins the security contract of the founder
// diagnostic endpoint: request-provided SQL is never executed; only fixed,
// allowlisted report keys reach the database.
func TestSuperAdminQuery_AllowlistOnly(t *testing.T) {
	db := newTestStore(t, "superadmin_query.db")
	// Seed one user so the "users" report has a row to return.
	if _, err := db.CreateUser(&models.User{
		OrgID: 1, Email: "founder@example.com", Name: "Founder",
		PasswordHash: "secret-hash", Role: models.RoleAdmin,
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	h := &Handler{deps: Deps{DB: db}}
	app := fiber.New()
	app.Post("/superadmin/query", h.superAdminQuery)

	t.Run("legacy raw sql is rejected and not executed", func(t *testing.T) {
		requireRawSQLRejected(t, app)
	})
	t.Run("unknown report key returns 400", func(t *testing.T) {
		requireUnknownReportRejected(t, app)
	})
	t.Run("allowed users report executes with expected shape", func(t *testing.T) {
		requireUsersReportResponse(t, app)
	})
	t.Run("allowed organizations report executes", func(t *testing.T) {
		requireOrganizationsReportResponse(t, app)
	})
}
