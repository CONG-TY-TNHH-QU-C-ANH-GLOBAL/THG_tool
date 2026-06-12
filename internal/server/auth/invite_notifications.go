package auth

import (
	"encoding/json"
	"log"

	"github.com/thg/scraper/internal/models"
)

// In-app notification emitters for the invite flow (PR-1). Kept out of
// onboarding.go (legacy large file) — handlers only insert call-sites.
// Failures are logged, never block the invite operation itself.

// notifyInviteReceived writes a personal notification for an invitee
// who ALREADY has an account, so the bell surfaces the invite without
// the email. New (unregistered) emails get the email link only.
func (h *Handler) notifyInviteReceived(orgID int64, orgName, inviterName, email, role, token string) {
	invitee, _ := h.deps.DB.GetUserByEmail(email)
	if invitee == nil {
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"token":        token,
		"org_name":     orgName,
		"role":         role,
		"inviter_name": inviterName,
	})
	if err := h.deps.DB.InsertNotification(
		orgID, invitee.ID, models.NotificationInviteReceived,
		"Bạn được mời tham gia workspace",
		inviterName+" đã mời bạn tham gia workspace "+orgName+" với vai trò "+role+".",
		string(payload),
	); err != nil {
		log.Printf("[InviteNotify] invite_received insert failed org=%d invitee=%d: %v", orgID, invitee.ID, err)
	}
}

// notifyInviteAccepted writes the org-wide (admin-visible) notification
// after an invitee joins.
func (h *Handler) notifyInviteAccepted(orgID int64, memberName, memberEmail, role string) {
	payload, _ := json.Marshal(map[string]string{
		"member_name":  memberName,
		"member_email": memberEmail,
		"role":         role,
	})
	if err := h.deps.DB.InsertNotification(
		orgID, 0, models.NotificationInviteAccepted,
		"Thành viên mới đã tham gia",
		memberName+" ("+memberEmail+") đã tham gia workspace với vai trò "+role+".",
		string(payload),
	); err != nil {
		log.Printf("[InviteNotify] invite_accepted insert failed org=%d: %v", orgID, err)
	}
}
