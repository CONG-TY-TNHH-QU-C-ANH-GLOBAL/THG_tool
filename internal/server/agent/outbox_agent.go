package agent

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/system"
)

// agentGetOutbox returns approved outbound messages for local execution.
// GET /api/agent/outbox
func (h *Handler) agentGetOutbox(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	agentID, _ := c.Locals("agent_id").(int64)
	assignedAccountID, _ := c.Locals("agent_assigned_account_id").(int64)
	workerID, _ := c.Locals("agent_token_fp").(string)
	if orgID <= 0 || agentID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	limit := c.QueryInt("limit", 5)
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	_ = h.db.ResetStaleSendingOutboundForOrg(orgID, 10*time.Minute)
	candidates, err := h.db.GetOutboundByStatusForOrg(orgID, string(models.OutboundApproved), limit*4)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	msgs := make([]models.OutboundMessage, 0, limit)
	for _, msg := range candidates {
		if len(msgs) >= limit {
			break
		}
		if msg.AccountID <= 0 {
			continue
		}
		if assignedAccountID > 0 && msg.AccountID != assignedAccountID {
			continue
		}
		ownsStream, err := h.db.ConnectorOwnsAccountStream(orgID, agentID, msg.AccountID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if !ownsStream {
			continue
		}
		if err := h.db.ClaimApprovedOutboundForOrg(orgID, msg.ID, workerID); err != nil {
			continue
		}
		msg.Status = models.OutboundSending
		msgs = append(msgs, msg)
	}
	return c.JSON(fiber.Map{"messages": msgs, "count": len(msgs)})
}

// agentOutboxSent marks an outbound message as sent (agent executed it successfully).
// POST /api/agent/outbox/:id/sent
func (h *Handler) agentOutboxSent(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.db.UpdateOutboundStatusForOrg(orgID, id, models.OutboundSent); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	if msg, err := h.db.GetOutboundForOrg(orgID, id); err == nil && msg.Type == "inbox" && msg.TargetURL != "" {
		threadID, threadErr := h.db.CreateThreadForOrg(orgID, 0, string(msg.Platform), msg.TargetURL, msg.TargetName, "")
		if threadErr == nil {
			_ = h.db.AddThreadMessage(threadID, "outbound", msg.Content, true)
		}
	}
	system.NotifyOutboundStatus(h.db, h.notifier, orgID, id, models.OutboundSent)
	return c.JSON(fiber.Map{"status": "sent"})
}

// agentOutboxFailed marks an outbound message as failed.
// POST /api/agent/outbox/:id/failed
func (h *Handler) agentOutboxFailed(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Error string `json:"error"`
	}
	_ = c.BodyParser(&req)
	if err := h.db.UpdateOutboundStatusForOrg(orgID, id, models.OutboundFailed); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	system.NotifyOutboundStatusDetail(h.db, h.notifier, orgID, id, models.OutboundFailed, req.Error)
	return c.JSON(fiber.Map{"status": "failed"})
}
