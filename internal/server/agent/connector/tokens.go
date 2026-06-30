package connector

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// agentCreateToken creates a new agent token (admin only, JWT auth).
// POST /api/admin/agent-tokens
func (h *Handler) agentCreateToken(c *fiber.Ctx) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	userID, _ := c.Locals("user_id").(int64)
	orgID, _ := c.Locals("org_id").(int64)
	id, plain, err := h.db.Connectors().CreateAgentToken(req.Name, userID, orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[Agent] Token created: %q (id=%d) by user %d", req.Name, id, userID)
	return c.Status(201).JSON(fiber.Map{
		"id":    id,
		"name":  req.Name,
		"token": plain, // shown once; client must copy immediately
	})
}

// agentListTokens lists all agent tokens (admin only, JWT auth).
// GET /api/admin/agent-tokens
func (h *Handler) agentListTokens(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	tokens, err := h.db.Connectors().ListAgentTokens(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"tokens": tokens, "count": len(tokens)})
}

// agentRevokeToken deactivates an agent token (admin only, JWT auth).
// DELETE /api/admin/agent-tokens/:id
func (h *Handler) agentRevokeToken(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.db.Connectors().RevokeAgentToken(id, orgID); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "agent token not found"})
	}
	return c.JSON(fiber.Map{"status": "revoked"})
}
