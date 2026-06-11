package integrations

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/telegram/control"
)

// channelStatus is the channel-neutral connector summary the UI renders. Facebook is active
// today; Taobao/1688 are modelled as planned so the UI never hardcodes a single channel.
type channelStatus struct {
	Channel string `json:"channel"`
	Label   string `json:"label"`
	Active  bool   `json:"active"`
}

func channelStatuses() []channelStatus {
	return []channelStatus{
		{Channel: "facebook", Label: "Facebook", Active: true},
		{Channel: "taobao", Label: "Taobao", Active: false},
		{Channel: "1688", Label: "1688", Active: false},
	}
}

// computeStatus derives the headline connection state from the org settings, binding counts, and
// process flags. Pure so it is unit-testable without a DB/HTTP context.
func computeStatus(enabled, botConfigured bool, activeBindings int) string {
	if !enabled && activeBindings == 0 {
		return "not_connected"
	}
	if !botConfigured || (enabled && activeBindings == 0) {
		return "needs_attention"
	}
	return "connected"
}

// getStatus returns the tenant's Telegram integration status for the settings page. Available to
// any authenticated org member (counts are non-sensitive; no tokens are ever returned).
func (h *Handler) getStatus(c *fiber.Ctx) error {
	orgID, _, _ := reqCtx(c)
	if orgID == 0 {
		return noOrg(c)
	}
	settings, err := h.deps.DB.Telegram().GetSettings(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "load settings failed"})
	}
	counts, err := h.deps.DB.Telegram().CountBindings(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "count bindings failed"})
	}
	destCount, err := h.deps.DB.Telegram().CountDestinations(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "count destinations failed"})
	}
	f := h.deps.Flags
	var webhookAt any
	if settings.WebhookLastAt.Valid {
		webhookAt = settings.WebhookLastAt.Time
	}
	return c.JSON(fiber.Map{
		// "connected" once there is at least one active connection — a channel DESTINATION
		// (primary) or a personal DM binding (secondary).
		"status":              computeStatus(settings.Enabled, f.BotConfigured, destCount+counts.Active),
		"active_destinations": destCount,
		"enabled":             settings.Enabled,
		"bot_username":        orFirst(settings.BotUsername, f.BotUsername),
		"bot_configured":      f.BotConfigured,
		"webhook_last_at":     webhookAt,
		"webhook_last_err":    settings.WebhookLastErr,
		"bound_users":         counts.Active,
		"alert_recipients":    counts.AlertRecipients,
		"actions_enabled":     f.ActionsEnabled,
		"flags": fiber.Map{
			"TELEGRAM_BOT_ENABLED":     f.BotEnabled,
			"TELEGRAM_NOTIFY_ENABLED":  f.NotifyEnabled,
			"TELEGRAM_ACTIONS_ENABLED": f.ActionsEnabled,
			"bot_token_configured":     f.BotConfigured,
		},
		"channels": channelStatuses(),
	})
}

func orFirst(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// enable turns the integration on for the org (admin). Audited.
func (h *Handler) enable(c *fiber.Ctx) error { return h.setEnabled(c, true) }

// disable turns the integration off for the org (admin). Audited.
func (h *Handler) disable(c *fiber.Ctx) error { return h.setEnabled(c, false) }

func (h *Handler) setEnabled(c *fiber.Ctx, enabled bool) error {
	orgID, userID, _ := reqCtx(c)
	if orgID == 0 {
		return noOrg(c)
	}
	if err := h.deps.DB.Telegram().SetEnabled(orgID, enabled); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "update failed"})
	}
	action := control.AuditIntegrationDisabled
	if enabled {
		action = control.AuditIntegrationEnabled
	}
	_ = h.deps.DB.Telegram().InsertAudit(orgID, userID, 0, action, "ok", "")
	return h.getStatus(c)
}
