package integrations

import (
	"encoding/json"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/telegram/control"
)

// Allow-lists + audit-event names come from the shared control package (single source of truth) —
// the REST API mirrors NOTHING locally.

// getAlerts returns the org's alert preferences (defaults when unset). Any org member may read.
func (h *Handler) getAlerts(c *fiber.Ctx) error {
	orgID, _, _ := reqCtx(c)
	if orgID == 0 {
		return noOrg(c)
	}
	prefs, err := h.deps.DB.Telegram().GetAlertPrefs(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "load alerts failed"})
	}
	var types []string
	_ = json.Unmarshal([]byte(prefs.AlertTypes), &types)
	if types == nil {
		types = []string{}
	}
	return c.JSON(fiber.Map{
		"alerts_enabled":    prefs.AlertsEnabled,
		"channel_filter":    prefs.ChannelFilter,
		"alert_types":       types,
		"available_types":   control.AlertTypes,
		"available_filters": control.ChannelFilters,
	})
}

// updateAlerts writes the org's alert preferences (admin). Validates against the shared allow-lists
// then audits the change.
func (h *Handler) updateAlerts(c *fiber.Ctx) error {
	orgID, userID, _ := reqCtx(c)
	if orgID == 0 {
		return noOrg(c)
	}
	var body struct {
		AlertsEnabled bool     `json:"alerts_enabled"`
		ChannelFilter string   `json:"channel_filter"`
		AlertTypes    []string `json:"alert_types"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	filter := control.NormalizeChannelFilter(body.ChannelFilter)
	if body.ChannelFilter != "" && !control.IsValidChannelFilter(body.ChannelFilter) {
		return c.Status(400).JSON(fiber.Map{"error": "invalid channel_filter"})
	}
	raw, _ := json.Marshal(control.SanitizeAlertTypes(body.AlertTypes))
	if err := h.deps.DB.Telegram().UpsertAlertPrefs(orgID, body.AlertsEnabled, filter, string(raw)); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "save alerts failed"})
	}
	_ = h.deps.DB.Telegram().InsertAudit(orgID, userID, 0, control.AuditAlertsUpdated, "ok", string(raw))
	return h.getAlerts(c)
}

// getAudit returns the org's recent Telegram audit events (admin).
func (h *Handler) getAudit(c *fiber.Ctx) error {
	orgID, _, _ := reqCtx(c)
	if orgID == 0 {
		return noOrg(c)
	}
	limit := 100
	if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 {
		limit = n
	}
	events, err := h.deps.DB.Telegram().ListAudit(orgID, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "load audit failed"})
	}
	return c.JSON(fiber.Map{"events": events})
}
