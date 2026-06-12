package auth

import (
	"time"

	"github.com/gofiber/fiber/v2"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
)

// Membership endpoints (SaaS UX Hardening PR-1).
//
// CONSTRAINT (documented per founder decision): membership is
// single-org-per-user TODAY — users.org_id is the whole model and an
// invite accept MOVES the user. These endpoints expose a LIST contract
// anyway so the frontend and future multi-membership storage can evolve
// without an API break. No join table is built yet (no overbuilding).

// refreshMembership handles POST /api/auth/refresh-membership.
// Re-reads org/role from the DB and issues a fresh token + cookies —
// the deterministic "my membership changed, make my session match"
// primitive the frontend calls after invite accept (and on
// SESSION_STALE responses from FreshOrgClaim). Idempotent.
func (h *Handler) refreshMembership(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int64)
	user, err := h.deps.DB.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	token, err := authpkg.GenerateAccessToken(userID, user.OrgID, user.Email, string(user.Role), h.deps.JWTSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}
	setAuthCookies(c, token, time.Now().Add(authpkg.AccessTokenTTL))
	return c.JSON(fiber.Map{
		"access_token": token,
		"org_id":       user.OrgID,
		"org_name":     h.orgNameOf(user.OrgID),
		"user": fiber.Map{
			"id":     user.ID,
			"org_id": user.OrgID,
			"email":  user.Email,
			"name":   user.Name,
			"role":   user.Role,
		},
	})
}

// listMemberships handles GET /api/auth/me/memberships.
func (h *Handler) listMemberships(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int64)
	user, err := h.deps.DB.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	type membership struct {
		OrgID   int64           `json:"org_id"`
		OrgName string          `json:"org_name"`
		Role    models.UserRole `json:"role"`
	}
	memberships := []membership{}
	if user.OrgID > 0 {
		memberships = append(memberships, membership{
			OrgID:   user.OrgID,
			OrgName: h.orgNameOf(user.OrgID),
			Role:    user.Role,
		})
	}
	return c.JSON(fiber.Map{"memberships": memberships, "count": len(memberships)})
}

// orgNameOf resolves an org display name; empty when unknown.
func (h *Handler) orgNameOf(orgID int64) string {
	if orgID <= 0 {
		return ""
	}
	org, err := h.deps.DB.GetOrganization(orgID)
	if err != nil || org == nil {
		return ""
	}
	return org.Name
}
