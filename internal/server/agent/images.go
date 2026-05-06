package agent

import "github.com/gofiber/fiber/v2"

// agentServeImage serves a local image file for the agent to download (for comment attachments).
// GET /api/agent/images?path=data/images/xxx.jpg
func (h *Handler) agentServeImage(c *fiber.Ctx) error {
	relPath := c.Query("path")
	if relPath == "" {
		return c.Status(400).JSON(fiber.Map{"error": "path required"})
	}
	// Sanitize: only allow paths starting with data/images/
	if len(relPath) < 12 || relPath[:12] != "data/images/" {
		return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
	}
	return c.SendFile(relPath)
}
