package auth

import (
	"encoding/json"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// Revoked invites disappear from pending lists and cannot be accepted;
// a wrong-email user can never accept someone else's invite.
// Fixture + doJSON live in invite_flow_test.go.
func TestInviteFlow_RevokedAndWrongEmail(t *testing.T) {
	f := newInviteFlowFixture(t, "invite_flow_guards.db")
	inviteeID, _ := f.db.CreateUser(&models.User{OrgID: 0, Email: "sale2@example.com", Name: "Sale 2", PasswordHash: "x", Role: models.RoleAdmin})
	strangerID, _ := f.db.CreateUser(&models.User{OrgID: 0, Email: "other@example.com", Name: "Other", PasswordHash: "x", Role: models.RoleAdmin})
	adminApp := f.app(f.adminID, f.orgID, "admin")

	_, created := doJSON(t, adminApp, "POST", "/org/invites", `{"email":"sale2@example.com","role":"sales"}`)
	token := created["token"].(string)
	inviteID := int64(created["id"].(float64))

	// Wrong email → 403.
	if code, _ := doJSON(t, f.app(strangerID, 0, "admin"), "POST", "/auth/join/"+token, ""); code != 403 {
		t.Fatalf("wrong-email accept status = %d, want 403", code)
	}

	// Revoke → invisible + not acceptable; admin list shows 'revoked'.
	if code, _ := doJSON(t, adminApp, "DELETE", "/org/invites/"+itoa(inviteID), ""); code != 200 {
		t.Fatalf("revoke failed")
	}
	if code, p := doJSON(t, f.app(inviteeID, 0, "admin"), "GET", "/auth/me/invites", ""); code != 200 || p["count"].(float64) != 0 {
		t.Fatalf("revoked invite still pending: %v", p)
	}
	if code, _ := doJSON(t, f.app(inviteeID, 0, "admin"), "POST", "/auth/join/"+token, ""); code != 404 {
		t.Fatalf("revoked invite acceptable, want 404")
	}
	_, list := doJSON(t, adminApp, "GET", "/org/invites", "")
	if list["invites"].([]any)[0].(map[string]any)["status"] != "revoked" {
		t.Fatalf("admin list status != revoked: %v", list)
	}

	// Cross-tenant: notifications never leak across users/orgs.
	if n, _ := f.db.ListNotificationsForUser(0, strangerID, false, 10); len(n) != 0 {
		t.Fatalf("stranger sees notifications: %v", n)
	}
}

func itoa(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
