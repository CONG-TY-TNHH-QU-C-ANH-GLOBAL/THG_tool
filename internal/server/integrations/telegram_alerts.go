package integrations

import (
	"encoding/json"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// validChannelFilters / validAlertTypes are the channel-neutral allow-lists the UI offers. Kept
// here (transport/validation) so the store stays free of policy.
var validChannelFilters = map[string]bool{"all": true, "facebook": true, "taobao": true, "1688": true}

var validAlertTypes = map[string]bool{
	"connector_offline":          true,
	"gate1_failure_spike":        true,
	"submitted_unverified_spike": true,
	"automation_paused":          true,
	"account_needs_attention":    true,
	"circuit_breaker_triggered":  true,
}

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
		"available_types":   alertTypeKeys(),
		"available_filters": []string{"all", "facebook", "taobao", "1688"},
	})
}

// updateAlerts writes the org's alert preferences (admin). Validates the channel filter + alert
// types against the allow-lists, then audits the change.
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
	if body.ChannelFilter == "" {
		body.ChannelFilter = "all"
	}
	if !validChannelFilters[body.ChannelFilter] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid channel_filter"})
	}
	clean := make([]string, 0, len(body.AlertTypes))
	for _, t := range body.AlertTypes {
		if validAlertTypes[t] {
			clean = append(clean, t)
		}
	}
	raw, _ := json.Marshal(clean)
	if err := h.deps.DB.Telegram().UpsertAlertPrefs(orgID, body.AlertsEnabled, body.ChannelFilter, string(raw)); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "save alerts failed"})
	}
	_ = h.deps.DB.Telegram().InsertAudit(orgID, userID, 0, "alerts_updated", "ok", string(raw))
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

func alertTypeKeys() []string {
	return []string{
		"connector_offline", "gate1_failure_spike", "submitted_unverified_spike",
		"automation_paused", "account_needs_attention", "circuit_breaker_triggered",
	}
}
