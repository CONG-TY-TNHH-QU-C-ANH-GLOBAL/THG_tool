package integrations

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store/telegram"
	"github.com/thg/scraper/internal/telegram/control"
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
	_ = h.deps.DB.Telegram().InsertAudit(orgID, userID, 0, control.AuditBindCodeGenerated, "ok", "")
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

// testNotification actually SENDS a test message to the caller's own active binding(s) via the
// shared control service (which audits the delivery result). Returns a typed reason on failure.
func (h *Handler) testNotification(c *fiber.Ctx) error {
	orgID, userID, _ := reqCtx(c)
	if orgID == 0 || userID == 0 {
		return noOrg(c)
	}
	if h.deps.Control == nil {
		return c.Status(503).JSON(fiber.Map{"error": "telegram_runtime_unavailable"})
	}
	ok, reason := h.deps.Control.TestNotify(orgID, userID)
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": reason})
	}
	return c.JSON(fiber.Map{"sent": true})
}
