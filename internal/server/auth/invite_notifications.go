package auth

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/thg/scraper/internal/mailer"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/telegram/render"
)

// In-app + Telegram + email notification emitters for the invite flow
// (PR-1 substrate, PR-8 channels). Kept out of onboarding.go (legacy
// large file) — handlers only insert call-sites. Every emitter is
// best-effort: failures are logged, never block the invite operation.

// notifyInviteReceived writes a personal notification for an invitee
// who ALREADY has an account, so the bell surfaces the invite without
// the email. New (unregistered) emails get the email link only.
// Also emits the invite_created Telegram channel event (PR-8).
func (h *Handler) notifyInviteReceived(orgID int64, orgName, inviterName, email, role, token string) {
	if h.deps.TgEvents != nil {
		_, _ = h.deps.TgEvents.NotifyEvent(orgID, "invite_created", "facebook",
			render.InviteCreated(orgName, email, role, time.Now().Add(7*24*time.Hour).Format("02-01-2006")))
	}
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
// after an invitee joins, emits the invite_accepted Telegram event, and
// emails the inviting admin (PR-8). inviteID resolves the inviter.
func (h *Handler) notifyInviteAccepted(orgID, inviteID int64, memberName, memberEmail, role string) {
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
	orgName := h.orgNameOf(orgID)
	if h.deps.TgEvents != nil {
		_, _ = h.deps.TgEvents.NotifyEvent(orgID, "invite_accepted", "facebook",
			render.InviteAccepted(orgName, memberName, memberEmail, role))
	}
	// Accepted-confirmation email to the inviter (async, best-effort).
	var inviterID int64
	if err := h.deps.DB.DB().QueryRow(
		`SELECT COALESCE(created_by, 0) FROM org_invites WHERE id = ?`, inviteID,
	).Scan(&inviterID); err != nil || inviterID <= 0 {
		return
	}
	inviter, _ := h.deps.DB.GetUserByID(inviterID)
	if inviter == nil || !h.deps.Mailer.Enabled() {
		return
	}
	cfg := h.deps.Mailer
	go func() {
		if err := mailer.SendInviteAccepted(context.Background(), cfg, inviter.Email, orgName, memberName, memberEmail, role); err != nil {
			log.Printf("[InviteNotify] accepted email to %s failed: %v", inviter.Email, err)
		}
	}()
}
