package auth

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// Pending invite != membership: an invited user who has NOT clicked
// «Đồng ý tham gia» has zero memberships — the invite appears only in
// the invite/notification surfaces, never as a workspace.
func TestMemberships_ExcludePendingInvites(t *testing.T) {
	f := newInviteFlowFixture(t, "membership_boundary.db")
	inviteeID, _ := f.db.CreateUser(&models.User{OrgID: 0, Email: "pending@example.com", Name: "Pending P", PasswordHash: "x", Role: models.RoleAdmin})

	adminApp := f.app(f.adminID, f.orgID, "admin")
	if code, _ := doJSON(t, adminApp, "POST", "/org/invites", `{"email":"pending@example.com","role":"sales"}`); code != 201 {
		t.Fatalf("create invite failed: %d", code)
	}

	inviteeApp := f.app(inviteeID, 0, "admin")
	code, ms := doJSON(t, inviteeApp, "GET", "/auth/me/memberships", "")
	if code != 200 || ms["count"].(float64) != 0 {
		t.Fatalf("pending invite leaked into memberships: %v", ms)
	}
	// The invite IS visible on the invite surface (where accept lives).
	code, pending := doJSON(t, inviteeApp, "GET", "/auth/me/invites", "")
	if code != 200 || pending["count"].(float64) != 1 {
		t.Fatalf("invite missing from the accept surface: %v", pending)
	}
	// Membership in the DB is untouched: org 0 → TenantReady blocks
	// every workspace endpoint until the explicit accept.
	if u, _ := f.db.GetUserByID(inviteeID); u.OrgID != 0 {
		t.Fatalf("invite creation granted membership: org=%d", u.OrgID)
	}
}

// Leave workspace: non-destructive self-exit. The last admin is
// blocked; with a second admin the leave succeeds, the login account
// survives orgless, and a fresh token is issued.
func TestLeaveWorkspace_LastAdminGuardAndDetach(t *testing.T) {
	f := newInviteFlowFixture(t, "leave_workspace.db")
	adminApp := f.app(f.adminID, f.orgID, "admin")

	// Solo admin cannot leave.
	code, body := doJSON(t, adminApp, "POST", "/auth/me/leave-workspace", "")
	if code != 409 || body["code"] != "LAST_ADMIN" {
		t.Fatalf("solo-admin leave = %d %v, want 409 LAST_ADMIN", code, body)
	}

	// Promote a second admin → leave succeeds.
	secondID, _ := f.db.CreateUser(&models.User{OrgID: f.orgID, Email: "admin2@example.com", Name: "Admin 2", PasswordHash: "x", Role: models.RoleAdmin})
	_ = secondID
	code, body = doJSON(t, adminApp, "POST", "/auth/me/leave-workspace", "")
	if code != 200 || body["access_token"] == "" || body["left_org_id"].(float64) != float64(f.orgID) {
		t.Fatalf("leave = %d %v", code, body)
	}
	u, _ := f.db.GetUserByID(f.adminID)
	if u == nil || u.OrgID != 0 {
		t.Fatalf("leaver must survive orgless, got %+v", u)
	}
	// Orgless ex-member has no memberships; the removed workspace is
	// unreachable (TenantReady denies org 0 callers).
	code, ms := doJSON(t, f.app(f.adminID, 0, "admin"), "GET", "/auth/me/memberships", "")
	if code != 200 || ms["count"].(float64) != 0 {
		t.Fatalf("ex-member memberships = %v", ms)
	}
}
