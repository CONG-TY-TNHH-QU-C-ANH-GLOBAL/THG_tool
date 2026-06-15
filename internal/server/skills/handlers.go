package skills

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/drivers/copilot"
	"github.com/thg/scraper/internal/store"
)

// Deps holds dependencies needed by the skills API handlers.
type Deps struct {
	DB    *store.Store
	Agent *copilot.Agent
}

// skillsList returns the catalog of every registered skill, with a
// per-org `enabled` flag. Used by the dashboard chat / Settings UI to
// render available capabilities.
//
// Returns 503 when the agent (and therefore the skill registry) is
// not initialised — typically when OPENAI_API_KEY is unset and the
// system is running without the AI orchestrator.
//
// GET /api/skills
func list(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if deps.Agent == nil {
			return c.Status(503).JSON(fiber.Map{"error": "ai agent not configured"})
		}
		registry := deps.Agent.SkillRegistry()
		if registry == nil {
			return c.Status(503).JSON(fiber.Map{"error": "skill registry not initialised"})
		}
		orgID, _ := c.Locals("org_id").(int64)
		catalog := registry.Catalog(c.Context(), deps.DB, orgID)
		return c.JSON(fiber.Map{
			"skills": catalog,
			"count":  len(catalog),
			"org_id": orgID,
		})
	}
}

// skillsAll returns the unfiltered catalog (all registered skills,
// every category). Admin-only — exposes capabilities other tenants
// have. Useful for the platform admin to inspect what's registered.
//
// GET /api/admin/skills
func all(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if deps.Agent == nil {
			return c.Status(503).JSON(fiber.Map{"error": "ai agent not configured"})
		}
		registry := deps.Agent.SkillRegistry()
		if registry == nil {
			return c.Status(503).JSON(fiber.Map{"error": "skill registry not initialised"})
		}
		registered := registry.All()
		out := make([]fiber.Map, 0, len(registered))
		for _, sk := range registered {
			out = append(out, fiber.Map{
				"id":              sk.ID,
				"title":           sk.Title,
				"description":     sk.Description,
				"category":        sk.Category,
				"outbound":        sk.Outbound,
				"needs_account":   sk.NeedsAccount,
				"default_enabled": sk.DefaultEnabled,
				"parameters":      sk.Parameters,
			})
		}
		return c.JSON(fiber.Map{"skills": out, "count": len(out)})
	}
}

// skillSetEnabled flips one skill's enable flag for the caller's org.
// Admin role required (the route group enforces this) — the action
// changes outbound automation surface area for the whole tenant.
//
// PUT /api/skills/:id/enable      body unused; route encodes intent
// PUT /api/skills/:id/disable
func setEnabled(deps Deps, enabled bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if deps.Agent == nil || deps.Agent.SkillRegistry() == nil {
			return c.Status(503).JSON(fiber.Map{"error": "skill registry not initialised"})
		}
		skillID := strings.TrimSpace(c.Params("id"))
		if skillID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "skill id is required"})
		}
		if deps.Agent.SkillRegistry().Get(skillID) == nil {
			return c.Status(404).JSON(fiber.Map{"error": "skill not registered"})
		}
		orgID, _ := c.Locals("org_id").(int64)
		userID, _ := c.Locals("user_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "org context required"})
		}
		if err := deps.DB.Prompts().SetOrgSkillEnabled(c.Context(), orgID, skillID, enabled, userID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"id": skillID, "enabled": enabled})
	}
}

// skillExecutions returns the audit feed of recent skill runs for the
// caller's org, newest first. Limited to 200 rows per request.
//
// GET /api/skills/executions?limit=50
func executions(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "org context required"})
		}
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		rows, err := deps.DB.Prompts().ListRecentSkillExecutions(c.Context(), orgID, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		out := make([]fiber.Map, 0, len(rows))
		for _, r := range rows {
			out = append(out, fiber.Map{
				"id":         r.ID,
				"skill_id":   r.SkillID,
				"source":     r.Source,
				"summary":    r.Summary,
				"success":    r.Success,
				"error":      r.Error,
				"args":       r.ArgsJSON,
				"created_at": r.At,
				"user_id":    r.UserID,
			})
		}
		return c.JSON(fiber.Map{"executions": out, "count": len(out)})
	}
}
