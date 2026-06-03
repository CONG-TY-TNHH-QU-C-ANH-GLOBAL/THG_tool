package workspace

import "github.com/gofiber/fiber/v2"

// getExecutionContext returns the caller's Default Account used for
// deterministic outbound routing (Organic Sales Network ExecutionContext).
// GET /api/execution-context
func (h *Handler) getExecutionContext(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	return c.JSON(fiber.Map{"default_account_id": h.db.GetUserDefaultAccount(orgID, userID)})
}

// setExecutionContext sets the caller's Default Account (must be owned by the
// caller). default_account_id=0 clears it. PUT /api/execution-context
func (h *Handler) setExecutionContext(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	var req struct {
		DefaultAccountID int64 `json:"default_account_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := h.db.SetUserDefaultAccount(orgID, userID, req.DefaultAccountID, role); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "default_account_id": req.DefaultAccountID})
}
