package agent

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

// Async comment reverify agent endpoints (spec: specs/COMMENT_ASYNC_REVERIFY.md, PR-A).
// The connector polls /reverify/claim for posts to re-check, re-opens each, searches the
// DOM for the comment by actor + normalized text, and reports the verdict to
// /reverify/result. A positive match makes the backend append the append-only correction.
// Agent-token authed; scoped to the connector's org + assigned account.

// agentReverifyClaim handles GET /api/agent/reverify/claim?limit=N.
func (h *Handler) agentReverifyClaim(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	accountID, _ := c.Locals("agent_assigned_account_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	limit := c.QueryInt("limit", 5)
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	jobs, err := h.db.Coordination().ClaimDueReverifies(c.Context(), orgID, accountID, time.Now(), limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"reverifies": jobs, "count": len(jobs)})
}

// agentReverifyResult handles POST /api/agent/reverify/result with body
// {"id": N, "found": bool, "comment_permalink": "...", "notes": "..."}.
func (h *Handler) agentReverifyResult(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	var body struct {
		ID               int64  `json:"id"`
		Found            bool   `json:"found"`
		CommentPermalink string `json:"comment_permalink"`
		Notes            string `json:"notes"`
	}
	if err := c.BodyParser(&body); err != nil || body.ID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "id required"})
	}
	corrected, err := h.db.Coordination().ApplyReverifyResult(c.Context(), orgID, body.ID, body.Found, body.CommentPermalink, body.Notes)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "corrected": corrected})
}
