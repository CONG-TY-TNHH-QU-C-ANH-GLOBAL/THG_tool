package org

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
)

// Company Identity FE form backend (Omnichannel Sales Copilot, PR-3 brand trust).
// Reads/writes the EXACT org-scoped user_context keys that ai.LoadProfileForOrg +
// ai.ResolveCompanyIdentity already consume, so the comment generator gets a
// grounded brand/website/contact/CTA. A DEDICATED endpoint (not the partial-update
// business-profile PUT) so an EMPTY value actually CLEARS the field — required by
// "field rỗng thì AI không được nêu".
//
// company_name      -> org:{id}:business_name
// website           -> org:{id}:business_website
// official_contact  -> org:{id}:official_contact
// primary_cta       -> org:{id}:primary_cta
// service_summary   -> org:{id}:services   (same field ResolveCompanyIdentity reads)

type companyIdentityDTO struct {
	CompanyName     string `json:"company_name"`
	Website         string `json:"website"`
	OfficialContact string `json:"official_contact"`
	PrimaryCTA      string `json:"primary_cta"`
	ServiceSummary  string `json:"service_summary"`
}

// getCompanyIdentity — GET /api/org/company-identity. Any org member may read
// their own workspace's identity (org isolation via c.Locals("org_id")).
func (h *Handler) getCompanyIdentity(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": msgWorkspaceContextRequired})
	}
	p := ai.LoadProfileForOrg(h.deps.DB, orgID)
	return c.JSON(companyIdentityDTO{
		CompanyName:     p.Name,
		Website:         p.Website,
		OfficialContact: p.OfficialContact,
		PrimaryCTA:      p.PrimaryCTA,
		ServiceSummary:  p.Services,
	})
}

// updateCompanyIdentity — PUT /api/org/company-identity, admin-only. FULL set: an
// empty value CLEARS the key (so the agent stops citing a removed website/contact).
func (h *Handler) updateCompanyIdentity(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": msgWorkspaceContextRequired})
	}
	var req companyIdentityDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": msgInvalidRequest})
	}
	fields := map[string]string{
		"business_name":    strings.TrimSpace(req.CompanyName),
		"business_website": strings.TrimSpace(req.Website),
		"official_contact": strings.TrimSpace(req.OfficialContact),
		"primary_cta":      strings.TrimSpace(req.PrimaryCTA),
		"services":         strings.TrimSpace(req.ServiceSummary),
	}
	for key, val := range fields {
		if err := h.deps.DB.Leads().SetContext(fmt.Sprintf("org:%d:%s", orgID, key), val); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}
	return c.JSON(req)
}
