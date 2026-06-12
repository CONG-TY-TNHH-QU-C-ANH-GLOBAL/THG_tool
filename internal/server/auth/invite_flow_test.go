package auth

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
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

type inviteFlowFixture struct {
	db      *store.Store
	h       *Handler
	orgID   int64
	adminID int64
}

func newInviteFlowFixture(t *testing.T, name string) *inviteFlowFixture {
	t.Helper()
	dst := storetest.CopyTemplate(t, bootstrapStore, name)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	orgID, err := db.CreateOrganization(&models.Organization{Name: "THG Fulfill", PlanTier: models.PlanFree, Active: true})
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	adminID, err := db.CreateUser(&models.User{OrgID: orgID, Email: "admin@example.com", Name: "Admin A", PasswordHash: "x", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	return &inviteFlowFixture{
		db:      db,
		h:       &Handler{deps: Deps{DB: db, JWTSecret: "test-secret"}},
		orgID:   orgID,
		adminID: adminID,
	}
}

// app builds a fiber app with auth Locals injected for the given principal.
func (f *inviteFlowFixture) app(userID, orgID int64, role string) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user_id", userID)
		c.Locals("org_id", orgID)
		c.Locals("user_role", role)
		return c.Next()
	})
	app.Post("/org/invites", f.h.createInvite)
	app.Get("/org/invites", f.h.listInvites)
	app.Delete("/org/invites/:id", f.h.revokeInvite)
	app.Get("/auth/me/invites", f.h.listMyPendingInvites)
	app.Get("/auth/me/memberships", f.h.listMemberships)
	app.Post("/auth/refresh-membership", f.h.refreshMembership)
	app.Post("/auth/join/:token", f.h.acceptInvite)
	return app
}

func doJSON(t *testing.T, app *fiber.App, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// The full PR-1 acceptance path: invite → in-app notification for the
// existing invitee → explicit accept → membership moves WITHOUT
// logout/login (fresh /auth/me + memberships reflect the new org) →
// admin sees 'accepted' status + the org-wide notification + audit.
func TestInviteFlow_AcceptWithoutLogout(t *testing.T) {
	f := newInviteFlowFixture(t, "invite_flow.db")
	inviteeID, err := f.db.CreateUser(&models.User{OrgID: 0, Email: "sale@example.com", Name: "Sale S", PasswordHash: "x", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create invitee: %v", err)
	}
	adminApp := f.app(f.adminID, f.orgID, "admin")
	inviteeApp := f.app(inviteeID, 0, "admin")

	// Admin creates the invite.
	code, created := doJSON(t, adminApp, "POST", "/org/invites", `{"email":"sale@example.com","role":"sales"}`)
	if code != 201 {
		t.Fatalf("create invite status = %d (%v)", code, created)
	}
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("invite token missing: %v", created)
	}

	// Existing invitee got the in-app invite notification (personal).
	notifs, err := f.db.ListNotificationsForUser(0, inviteeID, false, 10)
	if err != nil || len(notifs) != 1 || notifs[0].Type != models.NotificationInviteReceived {
		t.Fatalf("invitee notifications = %v err=%v, want 1 workspace_invite_received", notifs, err)
	}

	// Invitee sees it on the pending list too.
	code, pending := doJSON(t, inviteeApp, "GET", "/auth/me/invites", "")
	if code != 200 || pending["count"].(float64) != 1 {
		t.Fatalf("pending invites = %v", pending)
	}

	// Explicit accept.
	code, accepted := doJSON(t, inviteeApp, "POST", "/auth/join/"+token, "")
	if code != 200 {
		t.Fatalf("accept status = %d (%v)", code, accepted)
	}
	if accepted["org_name"] != "THG Fulfill" || accepted["role"] != "sales" {
		t.Fatalf("accept response missing org_name/role: %v", accepted)
	}

	// Membership moved in the DB — the freshness source /auth/me reads.
	moved, _ := f.db.GetUserByID(inviteeID)
	if moved.OrgID != f.orgID || moved.Role != models.RoleSales {
		t.Fatalf("user after accept: org=%d role=%s", moved.OrgID, moved.Role)
	}

	// Memberships + refresh-membership reflect the new org immediately.
	memberApp := f.app(inviteeID, f.orgID, "sales")
	code, ms := doJSON(t, memberApp, "GET", "/auth/me/memberships", "")
	if code != 200 || ms["count"].(float64) != 1 {
		t.Fatalf("memberships = %v", ms)
	}
	code, rm := doJSON(t, memberApp, "POST", "/auth/refresh-membership", "")
	if code != 200 || rm["org_id"].(float64) != float64(f.orgID) || rm["access_token"] == "" {
		t.Fatalf("refresh-membership = %d %v", code, rm)
	}

	// Admin: invite shows 'accepted' + org-wide notification exists.
	code, list := doJSON(t, adminApp, "GET", "/org/invites", "")
	if code != 200 {
		t.Fatalf("list invites status = %d", code)
	}
	invites := list["invites"].([]any)
	if len(invites) != 1 || invites[0].(map[string]any)["status"] != "accepted" {
		t.Fatalf("admin invite list = %v, want status accepted", invites)
	}
	adminNotifs, _ := f.db.ListNotificationsForUser(f.orgID, f.adminID, true, 10)
	found := false
	for _, n := range adminNotifs {
		if n.Type == models.NotificationInviteAccepted {
			found = true
		}
	}
	if !found {
		t.Fatalf("admin org notifications missing workspace_invite_accepted: %v", adminNotifs)
	}
}

