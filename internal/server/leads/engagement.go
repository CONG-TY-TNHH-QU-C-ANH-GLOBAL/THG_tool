package leads

import (
	"context"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
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
		ctx := context.Background()
		state, err := deps.DB.Leads().GetLeadEngagement(ctx, orgID, id)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		// §6: explain comment eligibility using the same gates as comment_all_leads.
		attachEligibility(ctx, deps.DB, orgID, map[int64]*models.LeadEngagementState{id: state})
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
		ids, err := parseLeadIDsCSV(raw)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		ctx := context.Background()
		states, err := deps.DB.Leads().GetLeadEngagementsBatch(ctx, orgID, ids)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		// §6: per-lead comment eligibility (workspace readiness computed once).
		attachEligibility(ctx, deps.DB, orgID, states)
		return c.JSON(fiber.Map{"engagements": states})
	}
}
