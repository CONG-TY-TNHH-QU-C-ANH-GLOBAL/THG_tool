package agent

import (
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// imageBaseDir is the only directory agentServeImage is allowed to serve from.
var imageBaseDir = filepath.Clean("data/images")

// agentServeImage serves a local image file for the agent to download (for comment attachments).
// GET /api/agent/images?path=data/images/xxx.jpg
func (h *Handler) agentServeImage(c *fiber.Ctx) error {
	relPath := c.Query("path")
	if relPath == "" {
		return c.Status(400).JSON(fiber.Map{"error": "path required"})
	}
	// Collapse any "../" segments first, then verify the resolved path is still
	// confined to data/images/. A raw prefix check (the previous approach) is
	// bypassable with "data/images/../../etc/passwd"; filepath.Clean resolves
	// the traversal so the prefix check below is meaningful.
	clean := filepath.Clean(relPath)
	if clean != imageBaseDir && !strings.HasPrefix(clean, imageBaseDir+string(filepath.Separator)) {
		return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
	}
	return c.SendFile(clean)
}
