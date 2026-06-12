package org

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
)

// Admin staff removal (membership-vulnerability fix): DELETE
// /auth/users/:id detaches the WORKSPACE membership only — the global
// user record + credentials survive, the leaver's accounts are
// assignment-paused, and the last admin can never be removed.
func TestAdminDeleteUser_DetachesWithoutDestroying(t *testing.T) {
	db := newTestStore(t, "remove_member.db")
	adminID, _ := db.CreateUser(&models.User{OrgID: 3, Email: "boss@example.com", Name: "Boss", PasswordHash: "hash-boss", Role: models.RoleAdmin})
	salesID, _ := db.CreateUser(&models.User{OrgID: 3, Email: "sale@example.com", Name: "Sale", PasswordHash: "hash-sale", Role: models.RoleSales})
	accID, _ := db.Identities().AddAccount(&models.Account{
		OrgID: 3, Platform: models.PlatformFacebook, Name: "Sale FB",
		AssignedUserID: salesID, Status: models.AccountActive,
	})

	h := &Handler{deps: Deps{DB: db}}
	app := fiber.New()
	app.Delete("/auth/users/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", adminID)
		c.Locals("org_id", int64(3))
		c.Locals("user_role", "admin")
		return h.adminDeleteUser(c)
	})
	del := func(id int64) (int, map[string]any) {
		resp, err := app.Test(httptest.NewRequest("DELETE", "/auth/users/"+strconv.FormatInt(id, 10), nil))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		out := map[string]any{}
		_ = json.Unmarshal(raw, &out)
		return resp.StatusCode, out
	}

	code, body := del(salesID)
	if code != 200 || body["status"] != "removed_from_workspace" {
		t.Fatalf("remove = %d %v", code, body)
	}
	// Global identity + credentials preserved; can join another workspace.
	u, err := db.GetUserByID(salesID)
	if err != nil || u == nil {
		t.Fatalf("removal destroyed the user record: %v", err)
	}
	if u.OrgID != 0 {
		t.Fatalf("removed user still in workspace: org=%d", u.OrgID)
	}
	var hash string
	_ = db.DB().QueryRow(`SELECT password_hash FROM users WHERE id = ?`, salesID).Scan(&hash)
	if hash != "hash-sale" {
		t.Fatalf("credentials must survive removal, got %q", hash)
	}
	if paused, _ := db.Identities().AccountAssignmentPaused(accID); !paused {
		t.Fatalf("removed member's account must be assignment-paused")
	}

	// Removed user is gone from the workspace staff list (no access path back
	// without a new invite — TenantReady denies org 0 callers).
	users, _ := db.ListUsers(3)
	for _, x := range users {
		if x.ID == salesID {
			t.Fatalf("removed user still listed in workspace")
		}
	}

	// Last admin cannot be removed: seed a second org with one admin and a
	// second non-admin caller... simpler: admin removing themselves is
	// blocked by the self-check; removing the ONLY admin via another path is
	// covered by store.ErrLastAdmin — exercise it through the handler with a
	// second org admin target.
	soloAdmin, _ := db.CreateUser(&models.User{OrgID: 9, Email: "solo@example.com", Name: "Solo", PasswordHash: "x", Role: models.RoleAdmin})
	appX := fiber.New()
	appX.Delete("/auth/users/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", int64(999999)) // platform-ish caller id, org 9 context
		c.Locals("org_id", int64(9))
		c.Locals("user_role", "admin")
		return h.adminDeleteUser(c)
	})
	resp, err := appX.Test(httptest.NewRequest("DELETE", "/auth/users/"+strconv.FormatInt(soloAdmin, 10), nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatalf("last-admin removal = %d, want 409", resp.StatusCode)
	}
	if u, _ := db.GetUserByID(soloAdmin); u.OrgID != 9 {
		t.Fatalf("blocked removal must not mutate membership")
	}
}
