package auth

import (
	"context"
	"encoding/json"
	"fmt"
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
func (h *Handler) notifyInviteReceived(orgID, inviteID int64, orgName, inviterName, email, role, token string) {
	if h.deps.TgEvents != nil {
		_, _ = h.deps.TgEvents.NotifyEvent(orgID, "invite_created", "facebook",
			render.InviteCreated(orgName, email, role, time.Now().Add(7*24*time.Hour).Format("02-01-2006")))
	}
	invitee, _ := h.deps.DB.GetUserByEmail(email)
	if invitee == nil {
		return
	}
	// invite_id links the card to its invite so the frontend can collapse
	// duplicates; backend resolution (org+user+type) is the durable guarantee.
	payload, _ := json.Marshal(map[string]string{
		"token":        token,
		"invite_id":    fmt.Sprintf("%d", inviteID),
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

// notifyInviteResolvedForInvitee runs on accept: it clears the invitee's now-stale
// "Bạn được mời…" card(s) for this workspace (incl. duplicates) and writes a
// no-CTA workspace_joined history notification so the bell reflects membership,
// not a pending invite. Best-effort — never blocks the join.
func (h *Handler) notifyInviteResolvedForInvitee(orgID, userID int64, orgName, role string) {
	if err := h.deps.DB.ResolveInviteNotificationsForUser(orgID, userID); err != nil {
		log.Printf("[InviteNotify] resolve invite_received failed org=%d user=%d: %v", orgID, userID, err)
	}
	if err := h.deps.DB.InsertNotification(
		orgID, userID, models.NotificationWorkspaceJoined,
		"Bạn đã tham gia workspace",
		"Bạn hiện là thành viên của workspace "+orgName+" với vai trò "+role+".",
		"{}",
	); err != nil {
		log.Printf("[InviteNotify] workspace_joined insert failed org=%d user=%d: %v", orgID, userID, err)
	}
}

// notifyInviteAccepted writes the org-wide (admin-visible) notification
// after an invitee joins, emits the invite_accepted Telegram event, and
// emails the inviting admin (PR-8). inviteID resolves the inviter.
func (h *Handler) notifyInviteAccepted(orgID, inviteID int64, memberName, memberEmail, role string) {
	orgName := h.orgNameOf(orgID)
	payload, _ := json.Marshal(map[string]string{
		"member_name":  memberName,
		"member_email": memberEmail,
		"role":         role,
	})
	if err := h.deps.DB.InsertNotification(
		orgID, 0, models.NotificationInviteAccepted,
		"Nhân viên đã tham gia workspace",
		memberName+" ("+memberEmail+") đã chấp nhận lời mời tham gia workspace "+orgName+" với vai trò "+role+".",
		string(payload),
	); err != nil {
		log.Printf("[InviteNotify] invite_accepted insert failed org=%d: %v", orgID, err)
	}
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
