package auth

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// RequireAuth validates the JWT and injects user context into Fiber locals.
// Sets: user_id (int64), user_email (string), user_role (string).
func RequireAuth(jwtSecret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := extractToken(c)
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "authentication required",
			})
		}
		claims, err := ValidateAccessToken(token, jwtSecret)
		if err != nil {
			if err == ErrExpiredToken {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "token expired",
					"code":  "TOKEN_EXPIRED",
				})
			}
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid token",
			})
		}
		c.Locals("user_id", claims.UserID)
		c.Locals("org_id", claims.OrgID)
		c.Locals("user_email", claims.Email)
		c.Locals("user_role", claims.Role)
		return c.Next()
	}
}

// RequireRole restricts a route to users with one of the specified roles.
// Must be chained after RequireAuth.
func RequireRole(roles ...string) fiber.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("user_role").(string)
		// superadmin passes ALL role checks
		if role == "superadmin" || allowed[role] {
			return c.Next()
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "insufficient permissions",
		})
	}
}

// extractToken reads the bearer token from Authorization header or access_token cookie.
func extractToken(c *fiber.Ctx) string {
	if auth := c.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return c.Cookies("access_token")
}
