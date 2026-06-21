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

	post := func(payload string) (int, map[string]any) {
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

	t.Run("legacy raw sql is rejected and not executed", func(t *testing.T) {
		code, body := post(`{"sql":"SELECT password_hash FROM users"}`)
		if code != 400 {
			t.Fatalf("raw sql status = %d, want 400", code)
		}
		if _, ok := body["rows"]; ok {
			t.Fatalf("raw sql must not be executed; got rows in response: %v", body)
		}
	})

	t.Run("unknown report key returns 400", func(t *testing.T) {
		code, body := post(`{"report":"passwords"}`)
		if code != 400 || body["error"] != "invalid report" {
			t.Fatalf("unknown report = %d %v, want 400 invalid report", code, body)
		}
	})

	t.Run("allowed users report executes with expected shape", func(t *testing.T) {
		code, body := post(`{"report":"users"}`)
		if code != 200 {
			t.Fatalf("users report = %d %v, want 200", code, body)
		}
		cols, _ := body["columns"].([]any)
		got := map[string]bool{}
		for _, col := range cols {
			got[col.(string)] = true
		}
		for _, want := range []string{"id", "email", "name", "role", "active", "org_id", "created_at"} {
			if !got[want] {
				t.Fatalf("users report columns missing %q; got %v", want, cols)
			}
		}
		if got["password_hash"] {
			t.Fatalf("users report must not expose password_hash")
		}
		if n, _ := body["count"].(float64); n < 1 {
			t.Fatalf("users report count = %v, want >= 1", body["count"])
		}
	})

	t.Run("allowed organizations report executes", func(t *testing.T) {
		code, body := post(`{"report":"organizations"}`)
		if code != 200 {
			t.Fatalf("organizations report = %d %v, want 200", code, body)
		}
		if _, ok := body["columns"]; !ok {
			t.Fatalf("organizations report missing columns: %v", body)
		}
	})
}
