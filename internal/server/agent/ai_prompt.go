package agent

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) aiPrompt(c *fiber.Ctx) error {
	var req struct {
		Prompt    string `json:"prompt"`
		AccountID int64  `json:"account_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if h.agent == nil || !h.agent.Available() {
		return c.Status(503).JSON(fiber.Map{"error": "AI agent not configured (check OPENAI_API_KEY)"})
	}

	prompt := strings.TrimSpace(req.Prompt)
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	// RBAC-1 skill-path enforcement: thread caller identity so skill executors
	// can verify account ownership before queueing outbound. Sales staff cannot
	// queue via accounts they don't own.
	response, err := h.agent.ProcessPromptForOrgWithUser(c.Context(), prompt, "dashboard", orgID, req.AccountID, userID, role)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"response": response})
}

func (h *Handler) aiHistory(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit <= 0 {
		limit = 20
	}
	orgID, _ := c.Locals("org_id").(int64)
	history, err := h.db.Prompts().GetPromptHistoryForOrg(orgID, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"history": history, "count": len(history)})
}

func (h *Handler) aiDeleteHistoryItem(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid history id"})
	}
	if err := h.db.Prompts().DeletePromptLogForOrg(orgID, id); err != nil {
		if err == sql.ErrNoRows {
			return c.Status(404).JSON(fiber.Map{"error": "history item not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "deleted_id": id})
}

func (h *Handler) aiDeleteHistory(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	deleted, err := h.db.Prompts().DeleteAllPromptLogsForOrg(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "deleted": deleted})
}
