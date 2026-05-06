package agent

import (
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/system"
)

func (h *Handler) getOutbox(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	status := c.Query("status", "")
	msgType := c.Query("type", "")
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	messages, err := h.db.GetOutboundByFilterForOrg(orgID, status, msgType, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	counts, _ := h.db.CountOutboundByStatusForOrg(orgID)
	return c.JSON(fiber.Map{"messages": messages, "count": len(messages), "counts": counts})
}

func (h *Handler) draftOutbound(c *fiber.Ctx) error {
	var req struct {
		Type       string `json:"type"` // comment, inbox
		AccountID  int64  `json:"account_id"`
		TargetURL  string `json:"target_url"`
		TargetName string `json:"target_name"`
		Content    string `json:"content"` // manual content (optional, AI generates if empty)
		Context    string `json:"context"` // original post for AI context
		Auto       bool   `json:"auto"`    // true = queue as approved for immediate agent execution
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Type == "" {
		req.Type = "comment"
	}
	if req.Type != "comment" && req.Type != "inbox" && req.Type != "group_post" && req.Type != "profile_post" {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported outbound type"})
	}
	orgID := c.Locals("org_id").(int64)
	if req.AccountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "account_id is required"})
	}
	if acct, err := h.db.GetAccountForOrg(req.AccountID, orgID); err != nil || acct == nil {
		return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
	}

	guard, err := h.db.CanQueueOutboundForOrg(orgID, req.Type, req.TargetURL, req.TargetURL, 24*time.Hour)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !guard.Allowed {
		return c.Status(409).JSON(fiber.Map{
			"error":       "outbound_blocked",
			"reason":      guard.Reason,
			"existing_id": guard.ExistingID,
		})
	}

	status := models.OutboundDraft
	if req.Auto {
		status = models.OutboundApproved
	}
	msg := &models.OutboundMessage{
		OrgID:      orgID,
		Type:       req.Type,
		Platform:   models.PlatformFacebook,
		AccountID:  req.AccountID,
		TargetURL:  req.TargetURL,
		TargetName: req.TargetName,
		Content:    req.Content,
		Context:    req.Context,
		Status:     status,
	}

	id, err := h.db.InsertOutboundMessage(msg)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if status == models.OutboundApproved && h.wsHub != nil {
		h.wsHub.NotifyOutboxReady(1)
	}
	system.NotifyOutboundQueued(h.db, h.notifier, orgID, req.AccountID, id, req.Type, status)
	return c.Status(201).JSON(fiber.Map{"message_id": id, "status": status})
}

func (h *Handler) approveOutbound(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.db.UpdateOutboundStatusForOrg(orgID, id, models.OutboundApproved); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	if h.wsHub != nil {
		h.wsHub.NotifyOutboxReady(1)
	}
	system.NotifyOutboundStatus(h.db, h.notifier, orgID, id, models.OutboundApproved)
	return c.JSON(fiber.Map{"status": "approved", "message": "Đã duyệt! Tin nhắn sẽ được gửi tự động."})
}

func (h *Handler) rejectOutbound(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.db.UpdateOutboundStatusForOrg(orgID, id, models.OutboundRejected); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	return c.JSON(fiber.Map{"status": "rejected"})
}

func (h *Handler) editOutbound(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
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
	if err := h.db.UpdateOutboundContentForOrg(orgID, id, req.Content); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	return c.JSON(fiber.Map{"status": "updated"})
}

func (h *Handler) deleteOutbound(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.db.DeleteOutboundForOrg(orgID, id); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	return c.JSON(fiber.Map{"status": "deleted"})
}

func (h *Handler) deleteAllOutboundComments(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	count, err := h.db.DeleteAllOutboundCommentsForOrg(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[API] Reset all outbound comments: %d deleted", count)
	return c.JSON(fiber.Map{"ok": true, "deleted": count})
}
