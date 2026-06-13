package auth

import (
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// The invite-notification lifecycle bug: after accept, the invitee's pending
// "Bạn được mời…" card(s) must stop being actionable (resolved/read), including
// duplicates for the same workspace, and a no-CTA workspace_joined history row
// must replace them. Shares the fixture/helpers from invite_flow_test.go.
func TestInviteAccept_ResolvesPendingInviteNotifications(t *testing.T) {
	f := newInviteFlowFixture(t, "invite_resolve.db")
	inviteeID, err := f.db.CreateUser(&models.User{OrgID: 0, Email: "sale@example.com", Name: "Sale S", PasswordHash: "x", Role: models.RoleSales})
	if err != nil {
		t.Fatalf("create invitee: %v", err)
	}
	adminApp := f.app(f.adminID, f.orgID, "admin")
	inviteeApp := f.app(inviteeID, 0, "sales")

	_, created := doJSON(t, adminApp, "POST", "/org/invites", `{"email":"sale@example.com","role":"sales"}`)
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("invite token missing: %v", created)
	}
	// Reproduce the reported screenshot: a SECOND pending card for the same
	// workspace (stale duplicate the bell was rendering with an accept CTA).
	if err := f.db.InsertNotification(f.orgID, inviteeID, models.NotificationInviteReceived,
		"Bạn được mời tham gia workspace", "dup", `{"token":"`+token+`"}`); err != nil {
		t.Fatalf("seed duplicate notification: %v", err)
	}
	if got := unreadOfType(t, f.db, inviteeID, models.NotificationInviteReceived); got < 2 {
		t.Fatalf("before accept want >=2 unread invite_received, got %d", got)
	}

	if code, out := doJSON(t, inviteeApp, "POST", "/auth/join/"+token, ""); code != 200 {
		t.Fatalf("accept status=%d (%v)", code, out)
	}

	// After accept: every invite_received card for this workspace is resolved
	// (dupes collapsed together) — the bell shows no pending accept CTA.
	if got := unreadOfType(t, f.db, inviteeID, models.NotificationInviteReceived); got != 0 {
		t.Fatalf("after accept want 0 unread invite_received, got %d", got)
	}
	// And exactly one no-CTA workspace_joined history notification is recorded.
	notifs, _ := f.db.ListNotificationsForUser(f.orgID, inviteeID, false, 20)
	joined := 0
	for _, n := range notifs {
		if n.Type == models.NotificationWorkspaceJoined {
			joined++
		}
	}
	if joined != 1 {
		t.Fatalf("want exactly 1 workspace_joined notification, got %d (%v)", joined, notifs)
	}
}

func unreadOfType(t *testing.T, db *store.Store, userID int64, ntype string) int {
	t.Helper()
	ns, err := db.ListNotificationsForUser(0, userID, false, 100)
	if err != nil {
		t.Fatalf("ListNotificationsForUser: %v", err)
	}
	n := 0
	for _, x := range ns {
		if x.Type == ntype && x.ReadAt == nil {
			n++
		}
	}
	return n
}
