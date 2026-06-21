package org

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
)

// getBusinessProfile handles GET /api/org/business-profile — returns the
// workspace's BusinessProfile as JSON. Any authenticated member of the org
// can read (tenant-scoped); writes are admin-only.
//
// Org isolation is enforced by the tenant_ready middleware that populates
// c.Locals("org_id") from the JWT — there is no way to pass an arbitrary
// org ID. A user in org A reading this endpoint always sees org A's
// profile, never org B's, even if they fabricate headers.
func (h *Handler) getBusinessProfile(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": msgWorkspaceContextRequired})
	}
	profile := ai.LoadProfileForOrg(h.deps.DB, orgID)
	return c.JSON(fiber.Map{
		"profile":       profile,
		"is_configured": profile.IsConfigured(),
	})
}

// updateBusinessProfile handles PUT /api/org/business-profile — admin
// only. Accepts a partial BusinessProfile JSON; empty fields are skipped
// (partial-update semantics matching updateOrg). Persists via
// BusinessProfile.SaveForOrg which writes the canonical org-scoped keys
// LoadProfileForOrg reads back.
func (h *Handler) updateBusinessProfile(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": msgWorkspaceContextRequired})
	}
	var req ai.BusinessProfile
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": msgInvalidRequest})
	}
	// Trim every string field so trailing whitespace from copy-paste does
	// not silently invalidate the "skip empty" check downstream.
	req.Name = strings.TrimSpace(req.Name)
	req.Industry = strings.TrimSpace(req.Industry)
	req.Description = strings.TrimSpace(req.Description)
	req.Services = strings.TrimSpace(req.Services)
	req.Targets = strings.TrimSpace(req.Targets)
	req.TargetAuthorRole = strings.ToLower(strings.TrimSpace(req.TargetAuthorRole))
	req.TargetSignals = strings.TrimSpace(req.TargetSignals)
	req.NegativeSignals = strings.TrimSpace(req.NegativeSignals)
	req.Location = strings.TrimSpace(req.Location)
	req.Markets = strings.TrimSpace(req.Markets)
	req.USP = strings.TrimSpace(req.USP)
	req.Tone = strings.TrimSpace(req.Tone)
	req.ApprovalPolicy = strings.TrimSpace(req.ApprovalPolicy)
	req.RejectRules = strings.TrimSpace(req.RejectRules)

	if err := req.SaveForOrg(h.deps.DB, orgID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	updated := ai.LoadProfileForOrg(h.deps.DB, orgID)
	return c.JSON(fiber.Map{
		"profile":       updated,
		"is_configured": updated.IsConfigured(),
	})
}
