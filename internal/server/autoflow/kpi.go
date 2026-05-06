package autoflow

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
)

func (h *Handler) autoflowGetStaff(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	staff, err := h.deps.DB.GetStaffWithKPI(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	type row struct {
		ID        int64  `json:"id"`
		OrgID     int64  `json:"org_id"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		Status    string `json:"status"`
		Joined    string `json:"joined"`
		Convs     int    `json:"convs"`
		Converted int    `json:"converted"`
		Cmts      int    `json:"cmts"`
		Pts       int    `json:"pts"`
	}
	out := make([]row, 0, len(staff))
	for _, m := range staff {
		status := "Active"
		if !m.Active {
			status = "Suspended"
		}
		out = append(out, row{
			ID: m.UserID, OrgID: m.OrgID, Name: m.Name, Email: m.Email, Role: m.Role,
			Status: status, Joined: m.Joined,
			Convs: m.Convs, Converted: m.Converted, Cmts: m.Cmts, Pts: m.Pts,
		})
	}
	return c.JSON(fiber.Map{"staff": out, "count": len(out)})
}

func (h *Handler) autoflowUpdateKPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	staffID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		Convs     *int `json:"convs"`
		Converted *int `json:"converted"`
		Cmts      *int `json:"cmts"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if err := h.deps.DB.UpsertStaffKPI(staffID, orgID, store.KPIDelta{
		Convs:     body.Convs,
		Converted: body.Converted,
		Cmts:      body.Cmts,
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// Ã¢â€â‚¬Ã¢â€â‚¬ KPI Config Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬

func (h *Handler) autoflowGetKPIConfig(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	cfg, err := h.deps.DB.GetKPIConfig(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"conv_pts":  cfg.ConvPts,
		"conv2_pts": cfg.Conv2Pts,
		"cmt_pts":   cfg.CmtPts,
		"bonus_pts": cfg.BonusPts,
		"bonus_amt": cfg.BonusAmt,
		"pen_pts":   cfg.PenPts,
		"pen_amt":   cfg.PenAmt,
	})
}

func (h *Handler) autoflowUpdateKPIConfig(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	var body struct {
		ConvPts  *int `json:"conv_pts"`
		Conv2Pts *int `json:"conv2_pts"`
		CmtPts   *int `json:"cmt_pts"`
		BonusPts *int `json:"bonus_pts"`
		BonusAmt *int `json:"bonus_amt"`
		PenPts   *int `json:"pen_pts"`
		PenAmt   *int `json:"pen_amt"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	existing, _ := h.deps.DB.GetKPIConfig(orgID)
	cfg := store.KPIConfig{OrgID: orgID, ConvPts: existing.ConvPts, Conv2Pts: existing.Conv2Pts, CmtPts: existing.CmtPts, BonusPts: existing.BonusPts, BonusAmt: existing.BonusAmt, PenPts: existing.PenPts, PenAmt: existing.PenAmt}
	if body.ConvPts != nil {
		cfg.ConvPts = *body.ConvPts
	}
	if body.Conv2Pts != nil {
		cfg.Conv2Pts = *body.Conv2Pts
	}
	if body.CmtPts != nil {
		cfg.CmtPts = *body.CmtPts
	}
	if body.BonusPts != nil {
		cfg.BonusPts = *body.BonusPts
	}
	if body.BonusAmt != nil {
		cfg.BonusAmt = *body.BonusAmt
	}
	if body.PenPts != nil {
		cfg.PenPts = *body.PenPts
	}
	if body.PenAmt != nil {
		cfg.PenAmt = *body.PenAmt
	}
	if err := h.deps.DB.UpsertKPIConfig(orgID, cfg); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// Ã¢â€â‚¬Ã¢â€â‚¬ Private Files Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬
