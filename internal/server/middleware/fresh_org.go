package middleware

import (
	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// FreshOrgClaim re-validates the JWT org claim against the users table
// on MUTATING requests (PR-2b stale-JWT mitigation).
//
// Why: org_id and role are static JWT claims. After an invite accept
// MOVES a user to a new org (single-org model: users.org_id is updated
// and a fresh token is issued), an older token remains valid until its
// TTL expires — up to 15 minutes in which the holder could still WRITE
// into the previous org. Reads are left alone (cheap, low blast
// radius); writes get one indexed users lookup and fail closed with a
// typed code the frontend uses to trigger a session refresh.
//
// Platform roles are exempt: they operate cross-org with org_id 0 by
// design (TenantReady already polices that combination).
func FreshOrgClaim(db *store.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		switch c.Method() {
		case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
			return c.Next()
		}
		userID, _ := c.Locals("user_id").(int64)
		claimOrg, _ := c.Locals("org_id").(int64)
		role, _ := c.Locals("user_role").(string)
		if userID <= 0 || models.IsPlatformRole(models.UserRole(role)) {
			return c.Next()
		}
		user, err := db.GetUserByID(userID)
		if err != nil || user == nil {
			return c.Status(401).JSON(fiber.Map{
				"error": "session user no longer exists",
				"code":  "SESSION_STALE",
			})
		}
		if user.OrgID != claimOrg {
			return c.Status(401).JSON(fiber.Map{
				"error": "your workspace membership changed — refresh your session",
				"code":  "SESSION_STALE",
			})
		}
		return c.Next()
	}
}
