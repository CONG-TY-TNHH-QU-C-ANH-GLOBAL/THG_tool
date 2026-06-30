package connector

import (
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

// PR-8 extension alert: a blocked build raises ONE staff notification +
// ONE admin notification (+ Telegram when configured), and the 24h
// throttle suppresses repeats — a 30s heartbeat can never spam.
func TestMaybeAlertExtensionOutdated_RateLimited(t *testing.T) {
	dst := storetest.CopyTemplate(t, bootstrapInputRBACStore, "extension_alerts.db")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const orgID = int64(77) // unique org so the package-level throttle map cannot collide
	staffID, _ := db.CreateUser(&models.User{OrgID: orgID, Email: "alert-staff@example.com", Name: "Alert Staff", PasswordHash: "x", Role: models.RoleSales})
	accID, _ := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: "Alert FB",
		AssignedUserID: staffID, Status: models.AccountActive,
	})
	var agentID int64
	res, err := db.DB().Exec(
		`INSERT INTO agent_tokens
			(org_id, name, created_by, token_hash, kind, transport, assigned_account_id,
			 fb_user_id, stream_status, version, active, last_seen, created_at)
		 VALUES (?, 'ext', ?, 'h-alert', 'extension_connector', 'chrome_extension', ?,
		        '111', 'facebook_logged_in', '0.5.10', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		orgID, staffID, accID)
	if err != nil {
		t.Fatalf("seed connector: %v", err)
	}
	agentID, _ = res.LastInsertId()

	h := &Handler{db: db} // tgEvents nil → Telegram skipped gracefully

	// Current build → no alert at all.
	h.maybeAlertExtensionOutdated(orgID, agentID+1000, "0.5.54")
	if n, _ := db.CountUnreadNotifications(orgID, staffID, false); n != 0 {
		t.Fatalf("current build must not alert, got %d", n)
	}

	// Blocked build → one staff + one admin notification.
	h.maybeAlertExtensionOutdated(orgID, agentID, "0.5.10")
	staffNotifs, _ := db.ListNotificationsForUser(orgID, staffID, false, 10)
	if len(staffNotifs) != 1 || staffNotifs[0].Type != models.NotificationExtensionUpdateRequired {
		t.Fatalf("staff notifications = %v", staffNotifs)
	}
	adminNotifs, _ := db.ListNotificationsForUser(orgID, 999, true, 10)
	if len(adminNotifs) != 1 {
		t.Fatalf("admin org-wide notifications = %v", adminNotifs)
	}

	// Immediate repeat (same connector) → throttled, no new rows.
	h.maybeAlertExtensionOutdated(orgID, agentID, "0.5.10")
	staffNotifs, _ = db.ListNotificationsForUser(orgID, staffID, false, 10)
	if len(staffNotifs) != 1 {
		t.Fatalf("throttle failed: %d staff notifications", len(staffNotifs))
	}
}
