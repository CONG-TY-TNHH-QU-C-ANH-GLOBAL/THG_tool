package auth

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

func (h *Handler) attachProvisionedOrgIfNeeded(user *models.User, ip string) (bool, error) {
	if user == nil || user.OrgID != 0 || models.IsPlatformRole(user.Role) {
		return false, nil
	}

	claim, err := h.deps.DB.FindProvisionedOrgByEmail(user.Email)
	if err != nil || claim == nil {
		return false, err
	}

	if err := h.deps.DB.UpdateUserOrg(user.ID, claim.OrgID, claim.Role); err != nil {
		return false, err
	}
	user.OrgID = claim.OrgID
	user.Role = claim.Role

	if err := h.completeProvisionedClaim(user.ID, claim, ip); err != nil {
		return false, err
	}
	return true, nil
}

func (h *Handler) completeProvisionedClaim(userID int64, claim *store.ProvisionedOrgClaim, ip string) error {
	if claim == nil || userID <= 0 || claim.OrgID <= 0 {
		return nil
	}
	if claim.Source == "invite" {
		if err := h.deps.DB.MarkInviteUsed(claim.InviteID, userID); err != nil {
			return err
		}
	}
	if err := h.deps.DB.UpsertStaffKPI(userID, claim.OrgID, store.KPIDelta{}); err != nil {
		return err
	}
	_ = h.deps.DB.InsertAuditLog(userID, "provisioned_workspace_claimed", ip,
		fmt.Sprintf(`{"org_id":%d,"role":%q,"source":%q,"invite_id":%d}`, claim.OrgID, claim.Role, claim.Source, claim.InviteID))
	return nil
}

func secureCookie(c *fiber.Ctx) bool {
	if strings.EqualFold(c.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	if strings.EqualFold(c.Protocol(), "https") {
		return true
	}
	return false
}
