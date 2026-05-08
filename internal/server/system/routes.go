package system

import "github.com/gofiber/fiber/v2"

// Routes registers the /api/system/* endpoints.
// These are public (no JWT required) — placed before the auth middleware.
func Routes(group fiber.Router, headless bool) {
	group.Get("/info", SystemInfo(headless))
	group.Get("/extension-beta-info", ServeExtensionBetaInfo())
	group.Get("/extension-beta-package", ServeExtensionBetaPackage())
}
