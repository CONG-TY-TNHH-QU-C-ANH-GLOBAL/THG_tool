package workspace

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// Admin assignment pause/resume (PR-2b). This is the admin SAFETY
// control over task assignment — deliberately NOT subject to the
// device-privacy gate (CanViewAccountDevice): an admin may pause a
// staff member's account without being able to view or operate the
// device itself. Both endpoints are mounted adminOnly.

// PUT /api/accounts/:id/pause
func (h *Handler) pauseAccountAssignment(c *fiber.Ctx) error {
	return h.setAccountAssignmentPause(c, true, "account_assignment_paused")
}

// PUT /api/accounts/:id/resume
func (h *Handler) resumeAccountAssignment(c *fiber.Ctx) error {
	return h.setAccountAssignmentPause(c, false, "account_assignment_resumed")
}

func (h *Handler) setAccountAssignmentPause(c *fiber.Ctx, paused bool, auditAction string) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid account id"})
	}
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	if err := h.db.Identities().SetAccountAssignmentPaused(id, orgID, paused); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}
	h.db.InsertAuditLog(userID, auditAction, c.IP(),
		fmt.Sprintf(`{"account_id":%d,"paused":%t}`, id, paused))
	return c.JSON(fiber.Map{
		"status":            "updated",
		"account_id":        id,
		"assignment_paused": paused,
	})
}
