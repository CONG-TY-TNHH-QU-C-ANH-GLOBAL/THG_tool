package server

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
)

func (s *Server) attachProvisionedOrgIfNeeded(user *models.User, ip string) (bool, error) {
	if user == nil || user.OrgID != 0 || user.Role == models.RoleSuperAdmin {
		return false, nil
	}

	claim, err := s.db.FindProvisionedOrgByEmail(user.Email)
	if err != nil || claim == nil {
		return false, err
	}

	if err := s.db.UpdateUserOrg(user.ID, claim.OrgID, claim.Role); err != nil {
		return false, err
	}
	user.OrgID = claim.OrgID
	user.Role = claim.Role

	_ = s.db.InsertAuditLog(user.ID, "provisioned_workspace_claimed", ip,
		fmt.Sprintf(`{"org_id":%d,"role":%q,"source":%q}`, claim.OrgID, claim.Role, claim.Source))
	return true, nil
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
