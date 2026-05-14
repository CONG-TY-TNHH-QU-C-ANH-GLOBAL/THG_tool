package leads

import (
	"context"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Coordination Plane PR-4: Lead Engagement State endpoints.
//
// These endpoints return read-only visibility data — the battlefield is
// shared (every staff sees every lead + every engagement). No access
// gates beyond the standard org-scoping that protected routes already
// enforce.

// getLeadEngagement handles GET /api/leads/:id/engagement.
// Returns the projected engagement state for one lead.
func getLeadEngagement(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
		}
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		state, err := deps.DB.GetLeadEngagement(context.Background(), orgID, id)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(state)
	}
}

// getLeadEngagementsBatch handles GET /api/leads/engagement?ids=1,2,3.
// Returns a map keyed by lead_id for list-view enrichment. Capped at
// 100 ids per call to keep the projection cheap.
func getLeadEngagementsBatch(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		raw := strings.TrimSpace(c.Query("ids", ""))
		if raw == "" {
			return c.JSON(fiber.Map{"engagements": map[string]any{}})
		}
		parts := strings.Split(raw, ",")
		if len(parts) > 100 {
			return c.Status(400).JSON(fiber.Map{"error": "max 100 ids per call"})
		}
		ids := make([]int64, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			id, err := strconv.ParseInt(p, 10, 64)
			if err != nil || id <= 0 {
				return c.Status(400).JSON(fiber.Map{"error": "invalid id: " + p})
			}
			ids = append(ids, id)
		}
		states, err := deps.DB.GetLeadEngagementsBatch(context.Background(), orgID, ids)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"engagements": states})
	}
}
