package integrations

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

// getBot returns the org's bot status — safe fields only. The token is NEVER returned.
func (h *Handler) getBot(c *fiber.Ctx) error {
	orgID, _, _ := reqCtx(c)
	if orgID == 0 {
		return noOrg(c)
	}
	cred, err := h.deps.Control.BotStatus(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "load bot failed"})
	}
	if cred == nil {
		return c.JSON(fiber.Map{"bot_configured": false, "platform_ready": true})
	}
	var verifiedAt *time.Time
	if cred.LastVerifiedAt.Valid {
		t := cred.LastVerifiedAt.Time
		verifiedAt = &t
	}
	// platform_ready=false means the stored token can't be decrypted by this runtime (internal
	// ENCRYPTION_KEY misconfiguration) — the UI shows an admin-config message, NOT a customer error.
	return c.JSON(fiber.Map{
		"bot_configured":   cred.Status == "active",
		"bot_username":     cred.BotUsername,
		"bot_display_name": cred.BotDisplayName,
		"token_last4":      cred.TokenLast4,
		"status":           cred.Status,
		"last_verified_at": verifiedAt,
		"last_error":       cred.LastError,
		"platform_ready":   h.deps.Control.EncryptionHealthy(orgID),
	})
}

// saveBot verifies a customer-supplied bot token via getMe and stores it ENCRYPTED. The token is
// read once from the body, never echoed back, never logged.
func (h *Handler) saveBot(c *fiber.Ctx) error {
	orgID, userID, _ := reqCtx(c)
	if orgID == 0 || userID == 0 {
		return noOrg(c)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if reason, ok := h.deps.Control.SaveBotToken(orgID, userID, body.Token); !ok {
		return c.Status(400).JSON(fiber.Map{"error": reason})
	}
	return h.getBot(c)
}

// verifyBot re-checks the stored token against getMe.
func (h *Handler) verifyBot(c *fiber.Ctx) error {
	orgID, userID, _ := reqCtx(c)
	if orgID == 0 {
		return noOrg(c)
	}
	if reason, ok := h.deps.Control.VerifyBot(orgID, userID); !ok {
		return c.Status(400).JSON(fiber.Map{"error": reason})
	}
	return h.getBot(c)
}

// deleteBot revokes (wipes) the stored bot token.
func (h *Handler) deleteBot(c *fiber.Ctx) error {
	orgID, userID, _ := reqCtx(c)
	if orgID == 0 {
		return noOrg(c)
	}
	if err := h.deps.Control.RevokeBot(orgID, userID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "revoke failed"})
	}
	return c.JSON(fiber.Map{"revoked": true})
}
