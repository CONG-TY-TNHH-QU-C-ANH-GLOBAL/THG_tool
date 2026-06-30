package outbox

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
)

// requireOutboundOwnerRow loads an outbound message and verifies the caller
// owns its target account. Admin / platform roles pass. Returns the message
// on success or writes a response and returns a non-nil error on failure.
func (h *Handler) requireOutboundOwnerRow(c *fiber.Ctx, orgID, userID int64, role string, id int64) (*models.OutboundMessage, error) {
	msg, err := h.db.GetOutboundForOrg(orgID, id)
	if err != nil || msg == nil {
		return nil, c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	if msg.AccountID <= 0 {
		// Legacy row with no account_id — only admin / platform may act.
		r := models.UserRole(role)
		if !models.IsPlatformRole(r) && r != models.RoleAdmin {
			return nil, c.Status(403).JSON(fiber.Map{"error": "you do not own this account"})
		}
		return msg, nil
	}
	if _, err := h.requireAccountOwner(h.db, c, msg.AccountID, orgID, userID, role); err != nil {
		return nil, err
	}
	return msg, nil
}

func (h *Handler) editOutbound(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if _, err := h.requireOutboundOwnerRow(c, orgID, userID, role, id); err != nil {
		return err
	}
	if err := h.db.UpdateOutboundContentForOrg(orgID, id, req.Content); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	return c.JSON(fiber.Map{"status": "updated"})
}

func (h *Handler) deleteOutbound(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if _, err := h.requireOutboundOwnerRow(c, orgID, userID, role, id); err != nil {
		return err
	}
	if err := h.db.DeleteOutboundForOrg(orgID, id); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	return c.JSON(fiber.Map{"status": "deleted"})
}

func (h *Handler) deleteAllOutboundComments(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	count, err := h.db.Leads().DeleteAllOutboundCommentsForOrg(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[API] Reset all outbound comments (org=%d): %d deleted", orgID, count)
	return c.JSON(fiber.Map{"ok": true, "deleted": count})
}

func (h *Handler) deleteAllOutboundPosts(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	count, err := h.db.Leads().DeleteAllOutboundPostsForOrg(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[API] Reset all outbound posts (org=%d): %d deleted", orgID, count)
	return c.JSON(fiber.Map{"ok": true, "deleted": count})
}
