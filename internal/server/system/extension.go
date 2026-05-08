package system

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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

type chromeExtensionManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	VersionName string `json:"version_name"`
}

func chromeExtensionPackageManifest(packagePath string) (chromeExtensionManifest, error) {
	zr, err := zip.OpenReader(packagePath)
	if err != nil {
		return chromeExtensionManifest{}, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "manifest.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return chromeExtensionManifest{}, err
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			return chromeExtensionManifest{}, err
		}
		var manifest chromeExtensionManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return chromeExtensionManifest{}, err
		}
		return manifest, nil
	}
	return chromeExtensionManifest{}, fmt.Errorf("manifest.json not found in extension package")
}

// ServeExtensionBetaInfo reports the exact beta zip currently being served so
// operators can verify whether Chrome is still running an old package.
func ServeExtensionBetaInfo() fiber.Handler {
	return func(c *fiber.Ctx) error {
		packageURL := strings.TrimSpace(os.Getenv("CHROME_EXTENSION_BETA_PACKAGE_URL"))
		if packageURL == "" {
			packageURL = "/api/system/extension-beta-package"
		}
		if !envFlagEnabled("CHROME_EXTENSION_BETA_ENABLED") {
			return c.JSON(fiber.Map{
				"enabled":     false,
				"package_url": packageURL,
			})
		}
		packagePath := filepath.Clean(chromeExtensionBetaPackagePath())
		info, err := os.Stat(packagePath)
		if err != nil || info.IsDir() {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "extension beta package not found"})
		}
		manifest, err := chromeExtensionPackageManifest(packagePath)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		version := strings.TrimSpace(manifest.VersionName)
		if version == "" {
			version = strings.TrimSpace(manifest.Version)
		}
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.JSON(fiber.Map{
			"enabled":     true,
			"name":        manifest.Name,
			"version":     version,
			"package_url": packageURL,
			"size_bytes":  info.Size(),
			"updated_at":  info.ModTime().UTC().Format(time.RFC3339),
		})
	}
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
		if manifest, err := chromeExtensionPackageManifest(packagePath); err == nil {
			version := strings.TrimSpace(manifest.VersionName)
			if version == "" {
				version = strings.TrimSpace(manifest.Version)
			}
			if version != "" {
				c.Set("X-THG-Extension-Version", version)
			}
		}
		c.Set(fiber.HeaderCacheControl, "no-store")
		c.Set(fiber.HeaderPragma, "no-cache")
		c.Set(fiber.HeaderExpires, "0")
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
