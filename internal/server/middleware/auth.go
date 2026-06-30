package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
)

// TenantReady blocks tenant APIs until a user has a valid org context.
func TenantReady() fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		role, _ := c.Locals("user_role").(string)
		if orgID == 0 && !models.IsPlatformRole(models.UserRole(role)) {
			return c.Status(403).JSON(fiber.Map{
				"error": "onboarding required",
				"code":  "ONBOARDING_REQUIRED",
			})
		}
		if orgID != 0 && models.IsPlatformRole(models.UserRole(role)) {
			return c.Status(403).JSON(fiber.Map{
				"error": "invalid platform role context",
			})
		}
		return c.Next()
	}
}
