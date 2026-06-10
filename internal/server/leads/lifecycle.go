package leads

import (
	"context"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
)

// Lead Lifecycle endpoints (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md, PR-4). Read-only
// projection for the dashboard's lifecycle tabs + filters, plus the manual archive/restore
// actions. Org-scoping is the standard protected-route guard; no extra access gate
// (battlefield model). Archiving never hard-deletes — it flips archived_at.

// getLeadLifecyclesBatch handles GET /api/leads/lifecycle?ids=1,2,3 — a map keyed by
// lead_id for list-view grouping. Capped at 100 ids per call.
func getLeadLifecyclesBatch(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		raw := strings.TrimSpace(c.Query("ids", ""))
		if raw == "" {
			return c.JSON(fiber.Map{"lifecycles": map[string]any{}})
		}
		parts := strings.Split(raw, ",")
		if len(parts) > 100 {
			return c.Status(400).JSON(fiber.Map{"error": "max 100 ids per call"})
		}
		ids := make([]int64, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p == "" {
				continue
			}
			id, err := strconv.ParseInt(p, 10, 64)
			if err != nil || id <= 0 {
				return c.Status(400).JSON(fiber.Map{"error": "invalid id: " + p})
			}
			ids = append(ids, id)
		}
		states, err := deps.DB.Leads().GetLeadLifecyclesBatch(context.Background(), orgID, ids, models.DefaultLeadLifecyclePolicy())
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"lifecycles": states})
	}
}

// getArchivedLeads handles GET /api/leads/archived?limit=&offset= — the "Đã lưu trữ" tab.
func getArchivedLeads(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		limit := c.QueryInt("limit", 50)
		offset := c.QueryInt("offset", 0)
		leads, err := deps.DB.Leads().ListArchivedLeads(context.Background(), orgID, limit, offset)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"leads": leads, "count": len(leads)})
	}
}

// archiveLead handles POST /api/leads/:id/archive with body {"reason": "..."}. An operator
// marking a lead not relevant is the manual_not_relevant path; any typed reason is accepted.
func archiveLead(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil || orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "invalid id or org context"})
		}
		var body struct {
			Reason string `json:"reason"`
		}
		_ = c.BodyParser(&body)
		reason := strings.TrimSpace(body.Reason)
		if reason == "" {
			reason = models.ArchiveReasonNotRelevant
		}
		if err := deps.DB.Leads().ArchiveLead(context.Background(), orgID, id, reason); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true, "archived": true, "reason": reason})
	}
}

// unarchiveLead handles POST /api/leads/:id/unarchive — restore to the live list.
func unarchiveLead(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil || orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "invalid id or org context"})
		}
		if err := deps.DB.Leads().UnarchiveLead(context.Background(), orgID, id); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true, "archived": false})
	}
}
