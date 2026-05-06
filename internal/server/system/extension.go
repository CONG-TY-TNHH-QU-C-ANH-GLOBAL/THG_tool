package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func envFlagEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func chromeExtensionBetaPackagePath() string {
	path := strings.TrimSpace(os.Getenv("CHROME_EXTENSION_BETA_PACKAGE_PATH"))
	if path == "" {
		path = "data/downloads/thg-chrome-extension.zip"
	}
	return path
}

func chromeExtensionStoreInfo() (string, string) {
	extensionID := strings.TrimSpace(os.Getenv("CHROME_EXTENSION_ID"))
	if extensionID == "" {
		extensionID = "nhalaldgpkoopgddccelckhaiegdbmfb"
	}
	storeURL := strings.TrimSpace(os.Getenv("CHROME_EXTENSION_STORE_URL"))
	if storeURL == "" && extensionID != "" {
		storeURL = fmt.Sprintf("https://chromewebstore.google.com/detail/thg-chrome-extension/%s", extensionID)
	}
	return storeURL, extensionID
}

func chromeExtensionBetaInfo() (string, string) {
	if !envFlagEnabled("CHROME_EXTENSION_BETA_ENABLED") {
		return "", ""
	}
	betaURL := strings.TrimSpace(os.Getenv("CHROME_EXTENSION_BETA_URL"))
	if betaURL == "" {
		betaURL = "/extension-beta"
	}
	packageURL := strings.TrimSpace(os.Getenv("CHROME_EXTENSION_BETA_PACKAGE_URL"))
	if packageURL == "" {
		packageURL = "/api/system/extension-beta-package"
	}
	return betaURL, packageURL
}

// ServeExtensionBetaPackage returns the CI-built Chrome Extension zip as a
// download. The file is read from disk with Cache-Control: no-store so
// browsers always get the latest version.
func ServeExtensionBetaPackage() fiber.Handler {
	return func(c *fiber.Ctx) error {
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
}

// SystemInfo returns server configuration for the frontend.
func SystemInfo(headless bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		storeURL, extensionID := chromeExtensionStoreInfo()
		resp := fiber.Map{
			"headless":                   headless,
			"chrome_extension_store_url": storeURL,
			"chrome_extension_id":        extensionID,
		}
		if betaURL, betaPackageURL := chromeExtensionBetaInfo(); betaURL != "" || betaPackageURL != "" {
			resp["chrome_extension_beta_url"] = betaURL
			resp["chrome_extension_beta_package_url"] = betaPackageURL
		}
		return c.JSON(resp)
	}
}
