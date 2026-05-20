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
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	if req.AccountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "account_id is required"})
	}
	// RBAC-1: execution-layer ownership. Sales staff can only queue outbound
	// against accounts they own. Admin / platform roles pass through.
	// See feedback_shared_battlefield_not_crm.md.
	if _, err := RequireAccountOwner(h.db, c, req.AccountID, orgID, userID, role); err != nil {
		return err
	}

	// Route through the canonical write path so the per-account dedup index +
	// action ledger (Coordination Plane PR-1) apply. Previously called
	// InsertOutboundMessage directly — that was the HTTP-bypass gap flagged
	// in project_outbound_audit_findings.md Critical #1.
	queueRes, err := h.db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:      orgID,
		Type:       req.Type,
		Platform:   models.PlatformFacebook,
		AccountID:  req.AccountID,
		TargetURL:  req.TargetURL,
		TargetName: req.TargetName,
		Content:    req.Content,
		Context:    req.Context,
	}, 24*time.Hour)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !queueRes.Decision.Allowed {
		return c.Status(409).JSON(fiber.Map{
			"error":       "outbound_blocked",
			"reason":      queueRes.Decision.Reason,
			"existing_id": queueRes.Decision.ExistingID,
		})
	}

	if queueRes.ExecutionState == models.ExecPlanned && h.wsHub != nil {
		h.wsHub.NotifyOutboxReady(1)
	}
	system.NotifyOutboundQueued(h.db, h.notifier, orgID, req.AccountID, queueRes.ID, req.Type, queueRes.ExecutionState)
	return c.Status(201).JSON(fiber.Map{
		"message_id":      queueRes.ID,
		"execution_state": string(queueRes.ExecutionState),
	})
}

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
	if _, err := RequireAccountOwner(h.db, c, msg.AccountID, orgID, userID, role); err != nil {
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
	count, err := h.db.DeleteAllOutboundCommentsForOrg(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[API] Reset all outbound comments (org=%d): %d deleted", orgID, count)
	return c.JSON(fiber.Map{"ok": true, "deleted": count})
}

func (h *Handler) deleteAllOutboundPosts(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	count, err := h.db.DeleteAllOutboundPostsForOrg(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[API] Reset all outbound posts (org=%d): %d deleted", orgID, count)
	return c.JSON(fiber.Map{"ok": true, "deleted": count})
}
