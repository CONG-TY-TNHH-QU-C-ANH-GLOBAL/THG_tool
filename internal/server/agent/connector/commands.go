package connector

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// agentConnectorCommands returns pending dashboard input commands for this Chrome Extension.
// GET /api/connectors/commands
func (h *Handler) agentConnectorCommands(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	limit := c.QueryInt("limit", 20)
	commands, err := h.db.Connectors().ClaimPendingConnectorCommands(orgID, agentID, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"commands": commands, "count": len(commands)})
}

// agentConnectorCommandDone marks a dashboard input command as executed by this Chrome Extension.
// POST /api/connectors/commands/:id/done
func (h *Handler) agentConnectorCommandDone(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid command id"})
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = c.BodyParser(&body)
	if err := h.db.Connectors().CompleteConnectorCommand(id, agentID, body.Error); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "command not found"})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}
