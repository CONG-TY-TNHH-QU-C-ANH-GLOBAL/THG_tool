package integrations

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store/telegram"
)

const bindCodeTTL = 10 * time.Minute

// createBindCode issues a one-time pairing code the caller sends to the bot as /bind <code>.
// Any authenticated org member may bind THEMSELVES. Audited.
func (h *Handler) createBindCode(c *fiber.Ctx) error {
	orgID, userID, _ := reqCtx(c)
	if orgID == 0 || userID == 0 {
		return noOrg(c)
	}
	code := telegram.GenerateCode(8)
	bc, err := h.deps.DB.Telegram().CreateBindCode(orgID, userID, code, bindCodeTTL)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "issue code failed"})
	}
	_ = h.deps.DB.Telegram().InsertAudit(orgID, userID, 0, "bind_code_generated", "ok", "")
	bot := h.deps.Flags.BotUsername
	deepLink := ""
	if bot != "" {
		deepLink = "https://t.me/" + bot + "?start=" + bc.Code
	}
	return c.Status(201).JSON(fiber.Map{
		"code":         bc.Code,
		"expires_at":   bc.ExpiresAt,
		"ttl_seconds":  int(bindCodeTTL.Seconds()),
		"bot_username": bot,
		"deep_link":    deepLink,
	})
}

// testNotification records a test-notification request against the caller's own active binding.
// Returns 400 when notifications are globally disabled or the caller has no active binding.
// Delivery itself is performed by the notifier (out of this control-plane PR); the result is
// recorded as "queued" so the audit trail is honest about what happened.
func (h *Handler) testNotification(c *fiber.Ctx) error {
	orgID, userID, _ := reqCtx(c)
	if orgID == 0 || userID == 0 {
		return noOrg(c)
	}
	if !h.deps.Flags.NotifyEnabled {
		return c.Status(400).JSON(fiber.Map{"error": "telegram_notify_disabled"})
	}
	mine, err := h.deps.DB.Telegram().ListBindingsByUser(orgID, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "load binding failed"})
	}
	if !hasActive(mine) {
		return c.Status(400).JSON(fiber.Map{"error": "no_active_binding"})
	}
	_ = h.deps.DB.Telegram().InsertAudit(orgID, userID, 0, "test_notification_sent", "queued", "")
	return c.JSON(fiber.Map{"queued": true, "note": "delivery pending notifier wiring"})
}

func hasActive(bs []telegram.Binding) bool {
	for _, b := range bs {
		if b.Status == "active" {
			return true
		}
	}
	return false
}
