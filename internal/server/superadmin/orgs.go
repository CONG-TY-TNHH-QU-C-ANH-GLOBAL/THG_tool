package superadmin

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
)

// listOrgs handles GET /api/superadmin/orgs — superadmin only.
func (h *Handler) listOrgs(c *fiber.Ctx) error {
	orgs, err := h.deps.DB.ListOrganizations()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"organizations": orgs, "count": len(orgs)})
}

// adminUpdateOrg handles PUT /api/superadmin/orgs/:id — superadmin changes plan/limits.
func (h *Handler) adminUpdateOrg(c *fiber.Ctx) error {
	id, _ := c.ParamsInt("id")
	var req struct {
		Name        string `json:"name"`
		Domain      string `json:"domain"`
		PlanTier    string `json:"plan_tier"`
		MaxAccounts int    `json:"max_accounts"`
		Active      *bool  `json:"active"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": msgInvalidRequest})
	}
	org, err := h.deps.DB.GetOrganization(int64(id))
	if err != nil || org == nil {
		return c.Status(404).JSON(fiber.Map{"error": "org not found"})
	}
	if req.Name != "" {
		org.Name = req.Name
	}
	if req.Domain != "" {
		org.Domain = req.Domain
	}
	if req.PlanTier != "" {
		org.PlanTier = models.PlanTier(req.PlanTier)
	}
	if req.MaxAccounts > 0 {
		org.MaxAccounts = req.MaxAccounts
	}
	if req.Active != nil {
		org.Active = *req.Active
	}
	if err := h.deps.DB.UpdateOrganization(org.ID, org.Name, org.Domain, org.PlanTier, org.MaxAccounts, org.Active); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "updated", "org": org})
}

func (h *Handler) superAdminDeleteOrg(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": msgInvalidID})
	}
	if id == 1 {
		return c.Status(403).JSON(fiber.Map{"error": "cannot delete platform org"})
	}
	if _, err := h.deps.DB.DB().ExecContext(c.Context(), `DELETE FROM organizations WHERE id = ?`, id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}
