package server

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (s *Server) serveExtensionBetaPackage(c *fiber.Ctx) error {
	if strings.ToLower(strings.TrimSpace(os.Getenv("CHROME_EXTENSION_BETA_ENABLED"))) != "true" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "extension beta lane is disabled"})
	}

	packagePath := strings.TrimSpace(os.Getenv("CHROME_EXTENSION_BETA_PACKAGE_PATH"))
	if packagePath == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "extension beta package is not configured"})
	}

	info, err := os.Stat(packagePath)
	if err != nil || info.IsDir() {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "extension beta package is unavailable"})
	}

	filename := filepath.Base(packagePath)
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		filename = "thg-chrome-extension.zip"
	}
	return c.Download(packagePath, filename)
}
