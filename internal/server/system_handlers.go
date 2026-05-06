package server

import (
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
)

func (s *Server) serveExtensionBetaPackage(c *fiber.Ctx) error {
	if !envFlagEnabled("CHROME_EXTENSION_BETA_ENABLED") {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "extension beta package is disabled"})
	}

	packagePath := filepath.Clean(chromeExtensionBetaPackagePath())
	info, err := os.Stat(packagePath)
	if err != nil || info.IsDir() {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "extension beta package not found"})
	}

	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderContentType, "application/zip")
	c.Attachment("thg-chrome-extension.zip")
	return c.SendFile(packagePath)
}
